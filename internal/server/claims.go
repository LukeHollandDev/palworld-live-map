package server

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/LukeHollandDev/palworld-live-map/internal/playerclaim"
)

const (
	claimSessionCookie     = "__Host-palworld_live_map_session"
	httpClaimSessionCookie = "palworld_live_map_http_session"
	claimCSRFHeader        = "X-Palworld-Live-Map"
	maxClaimBody           = 8 << 10
	maxClaimPlayerID       = 256
)

type startClaimRequest struct {
	PlayerID string `json:"playerId"`
}

type verifyClaimRequest struct {
	ChallengeToken string `json:"challengeToken"`
}

type claimErrorResponse struct {
	Error string `json:"error"`
}

type claimVerificationResponse struct {
	Status            playerclaim.VerificationStatus `json:"status"`
	Instructions      *playerclaim.Instructions      `json:"instructions,omitempty"`
	ExpiresAt         time.Time                      `json:"expiresAt,omitzero"`
	IdleExpiresAt     time.Time                      `json:"idleExpiresAt,omitzero"`
	AbsoluteExpiresAt time.Time                      `json:"absoluteExpiresAt,omitzero"`
}

type claimProgressDomain struct {
	ID           string   `json:"id"`
	Coverage     string   `json:"coverage"`
	CompletedIDs []string `json:"completedIds"`
	Total        int      `json:"total"`
}

type claimProgressResponse struct {
	SnapshotAt       time.Time             `json:"snapshotAt"`
	CatalogueVersion string                `json:"catalogueVersion"`
	Domains          []claimProgressDomain `json:"domains"`
}

func (s *Server) startPlayerClaim(w http.ResponseWriter, r *http.Request) {
	privateResponse(w)
	if !s.validClaimMutation(w, r) {
		return
	}
	if !s.claimStartLimiter.Allow(claimSource(r, s.settings.playerClaimsTrustedProxies), time.Now()) {
		writeClaimRateLimit(w, r)
		return
	}
	var request startClaimRequest
	if !decodeClaimJSON(w, r, &request) || !validPublicPlayerID(request.PlayerID) {
		writeJSON(w, r, http.StatusBadRequest, claimErrorResponse{Error: "invalid_request"})
		return
	}
	if !s.isPublicLivePlayer(request.PlayerID) {
		// Claims mode intentionally keeps offline save records private. Requiring
		// the target to be in the same live projection already visible to this
		// caller prevents this endpoint becoming an offline-roster oracle.
		s.writeStartClaimError(w, r, playerclaim.ErrUnavailable)
		return
	}
	if !s.acquireClaimWork() {
		writeClaimRateLimit(w, r)
		return
	}
	defer s.releaseClaimWork()
	challenge, err := s.claims.Start(r.Context(), request.PlayerID)
	if err != nil {
		s.writeStartClaimError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, challenge)
}

func (s *Server) isPublicLivePlayer(publicPlayerID string) bool {
	players, _, _ := s.source.PlayerSnapshotSince(0)
	for _, player := range players.Players {
		if player.Online && player.ID == publicPlayerID {
			return true
		}
	}
	return false
}

func (s *Server) verifyPlayerClaim(w http.ResponseWriter, r *http.Request) {
	privateResponse(w)
	if !s.validClaimMutation(w, r) {
		return
	}
	if !s.claimVerifyLimiter.Allow(claimSource(r, s.settings.playerClaimsTrustedProxies), time.Now()) {
		writeClaimRateLimit(w, r)
		return
	}
	var request verifyClaimRequest
	if !decodeClaimJSON(w, r, &request) || !validClaimBearer(request.ChallengeToken) {
		writeJSON(w, r, http.StatusBadRequest, claimErrorResponse{Error: "invalid_request"})
		return
	}
	if !s.acquireClaimWork() {
		writeClaimRateLimit(w, r)
		return
	}
	defer s.releaseClaimWork()
	verification, err := s.claims.Verify(r.Context(), request.ChallengeToken)
	if err != nil {
		if errors.Is(err, playerclaim.ErrVerificationInFlight) {
			writeJSON(w, r, http.StatusAccepted, claimVerificationResponse{Status: playerclaim.VerificationPending})
			return
		}
		if errors.Is(err, playerclaim.ErrChallengeNotFound) || errors.Is(err, playerclaim.ErrChallengeExpired) {
			writeJSON(w, r, http.StatusUnauthorized, claimErrorResponse{Error: "invalid_or_expired_challenge"})
			return
		}
		writeJSON(w, r, http.StatusServiceUnavailable, claimErrorResponse{Error: "claim_unavailable"})
		return
	}
	if verification.Status == playerclaim.VerificationArming ||
		verification.Status == playerclaim.VerificationPending ||
		verification.Status == playerclaim.VerificationReady {
		writeJSON(w, r, http.StatusAccepted, claimVerificationResponse{
			Status: verification.Status, Instructions: verification.Instructions, ExpiresAt: verification.ExpiresAt,
		})
		return
	}
	if verification.Status != playerclaim.VerificationVerified || verification.Session == nil || !validClaimBearer(verification.Session.Bearer) {
		writeJSON(w, r, http.StatusServiceUnavailable, claimErrorResponse{Error: "claim_unavailable"})
		return
	}
	s.setClaimCookie(w, verification.Session.Bearer, verification.Session.AbsoluteExpiresAt)
	writeJSON(w, r, http.StatusOK, claimVerificationResponse{
		Status:            verification.Status,
		IdleExpiresAt:     verification.Session.IdleExpiresAt,
		AbsoluteExpiresAt: verification.Session.AbsoluteExpiresAt,
	})
}

func (s *Server) claimSession(w http.ResponseWriter, r *http.Request) {
	privateResponse(w)
	principal, ok := s.claimPrincipal(w, r)
	if !ok {
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{
		"authenticated":     true,
		"playerId":          principal.PublicPlayerID(),
		"idleExpiresAt":     principal.IdleExpiresAt(),
		"absoluteExpiresAt": principal.AbsoluteExpiresAt(),
	})
}

func (s *Server) claimProgress(w http.ResponseWriter, r *http.Request) {
	privateResponse(w)
	principal, ok := s.claimPrincipal(w, r)
	if !ok {
		return
	}
	if !s.claimVerifyLimiter.Allow(claimSource(r, s.settings.playerClaimsTrustedProxies), time.Now()) {
		writeClaimRateLimit(w, r)
		return
	}
	if !s.acquireClaimWork() {
		writeClaimRateLimit(w, r)
		return
	}
	defer s.releaseClaimWork()
	progress, err := s.claims.Progress(r.Context(), principal)
	if err != nil {
		writeJSON(w, r, http.StatusServiceUnavailable, claimErrorResponse{Error: "progress_unavailable"})
		return
	}
	writeJSON(w, r, http.StatusOK, s.projectClaimProgress(progress))
}

func (s *Server) claimPrincipal(w http.ResponseWriter, r *http.Request) (playerclaim.Principal, bool) {
	cookie, err := r.Cookie(s.claimCookieName())
	if err != nil || !validClaimBearer(cookie.Value) {
		writeJSON(w, r, http.StatusUnauthorized, claimErrorResponse{Error: "authentication_required"})
		return playerclaim.Principal{}, false
	}
	principal, err := s.claims.ValidateSession(cookie.Value)
	if err != nil {
		s.clearClaimCookie(w)
		writeJSON(w, r, http.StatusUnauthorized, claimErrorResponse{Error: "authentication_required"})
		return playerclaim.Principal{}, false
	}
	return principal, true
}

func (s *Server) projectClaimProgress(progress playerclaim.PrivateProgress) claimProgressResponse {
	type domainSource struct {
		id       string
		category string
		keys     []string
	}
	sources := []domainSource{
		{id: "waypoints", category: "waypoints", keys: progress.FastTravelKeys},
		{id: "journals", category: "journals", keys: progress.NoteKeys},
	}
	domains := make([]claimProgressDomain, 0, len(sources))
	for _, source := range sources {
		completedKeys := make(map[string]struct{}, len(source.keys))
		for _, key := range source.keys {
			completedKeys[strings.ToLower(key)] = struct{}{}
		}
		completedIDs := make([]string, 0, len(completedKeys))
		total := 0
		for _, record := range s.worldCatalogue.Completion {
			if record.Category != source.category {
				continue
			}
			total++
			if _, complete := completedKeys[record.StateKey]; complete {
				completedIDs = append(completedIDs, record.LocationID)
			}
		}
		sort.Strings(completedIDs)
		domains = append(domains, claimProgressDomain{
			ID: source.id, Coverage: "complete", CompletedIDs: completedIDs, Total: total,
		})
	}
	return claimProgressResponse{
		SnapshotAt: progress.SnapshotAt.UTC(), CatalogueVersion: s.worldCatalogue.ContentHash, Domains: domains,
	}
}

func (s *Server) logoutClaimSession(w http.ResponseWriter, r *http.Request) {
	privateResponse(w)
	if !s.validClaimMutation(w, r) {
		return
	}
	if cookie, err := r.Cookie(s.claimCookieName()); err == nil && validClaimBearer(cookie.Value) {
		s.claims.RevokeSession(cookie.Value)
	}
	s.clearClaimCookie(w)
	writeJSON(w, r, http.StatusOK, map[string]bool{"authenticated": false})
}

func (s *Server) writeStartClaimError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, playerclaim.ErrInvalidTarget):
		writeJSON(w, r, http.StatusBadRequest, claimErrorResponse{Error: "invalid_request"})
	case errors.Is(err, playerclaim.ErrStoreFull):
		w.Header().Set("Retry-After", "60")
		writeJSON(w, r, http.StatusTooManyRequests, claimErrorResponse{Error: "claim_unavailable"})
	default:
		// Unknown players, stale rosters, decoder failures, and invalid private
		// evidence deliberately share one response to avoid an enumeration oracle.
		writeJSON(w, r, http.StatusServiceUnavailable, claimErrorResponse{Error: "claim_unavailable"})
	}
}

func (s *Server) validClaimMutation(w http.ResponseWriter, r *http.Request) bool {
	site := r.Header.Get("Sec-Fetch-Site")
	if r.Header.Get("Origin") != s.settings.playerClaimsOrigin ||
		site != "same-origin" ||
		r.Header.Get(claimCSRFHeader) != "1" {
		writeJSON(w, r, http.StatusForbidden, claimErrorResponse{Error: "request_rejected"})
		return false
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeJSON(w, r, http.StatusUnsupportedMediaType, claimErrorResponse{Error: "invalid_request"})
		return false
	}
	return true
}

func decodeClaimJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxClaimBody)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return false
	}
	return true
}

func validPublicPlayerID(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len(value) > maxClaimPlayerID || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validClaimBearer(value string) bool {
	if len(value) != 43 {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func (s *Server) claimCookieName() string {
	if s.settings.playerClaimsHTTP {
		return httpClaimSessionCookie
	}
	return claimSessionCookie
}

func (s *Server) setClaimCookie(w http.ResponseWriter, bearer string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name: s.claimCookieName(), Value: bearer, Path: "/", Expires: expiresAt.UTC(),
		Secure: !s.settings.playerClaimsHTTP, HttpOnly: true, SameSite: http.SameSiteStrictMode,
	})
}

func (s *Server) clearClaimCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: s.claimCookieName(), Value: "", Path: "/", MaxAge: -1, Expires: time.Unix(1, 0).UTC(),
		Secure: !s.settings.playerClaimsHTTP, HttpOnly: true, SameSite: http.SameSiteStrictMode,
	})
}

func privateResponse(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Pragma", "no-cache")
}

type claimLimitWindow struct {
	started time.Time
	count   int
}

// claimRequestLimiter bounds decoder work per resolved client network and
// process-wide. Forwarding headers are used only through explicitly trusted
// proxy CIDRs.
type claimRequestLimiter struct {
	mu             sync.Mutex
	perSource      map[string]claimLimitWindow
	perSourceLimit int
	perSourceSpan  time.Duration
	global         claimLimitWindow
	globalLimit    int
	globalSpan     time.Duration
}

func newClaimRequestLimiter(perSourceLimit int, perSourceSpan time.Duration, globalLimit int, globalSpan time.Duration) *claimRequestLimiter {
	return &claimRequestLimiter{
		perSource: make(map[string]claimLimitWindow), perSourceLimit: perSourceLimit, perSourceSpan: perSourceSpan,
		globalLimit: globalLimit, globalSpan: globalSpan,
	}
}

func (l *claimRequestLimiter) Allow(source string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.global.started.IsZero() || now.Sub(l.global.started) >= l.globalSpan || now.Before(l.global.started) {
		l.global = claimLimitWindow{started: now}
	}
	if l.global.count >= l.globalLimit {
		return false
	}
	window, exists := l.perSource[source]
	if !exists || now.Sub(window.started) >= l.perSourceSpan || now.Before(window.started) {
		if len(l.perSource) >= 2_048 {
			for key, candidate := range l.perSource {
				if now.Sub(candidate.started) >= l.perSourceSpan || now.Before(candidate.started) {
					delete(l.perSource, key)
				}
			}
			if len(l.perSource) >= 2_048 && !exists {
				return false
			}
		}
		window = claimLimitWindow{started: now}
	}
	if window.count >= l.perSourceLimit {
		return false
	}
	window.count++
	l.perSource[source] = window
	l.global.count++
	return true
}

func claimSource(r *http.Request, trustedProxies []netip.Prefix) string {
	address, ok := remoteIP(r.RemoteAddr)
	if !ok {
		return "invalid"
	}
	if trustedProxy(address, trustedProxies) {
		header := r.Header.Get("X-Forwarded-For")
		if len(header) > 2_048 || strings.Count(header, ",") >= 32 {
			return claimNetwork(address)
		}
		forwarded := strings.Split(header, ",")
		for index := len(forwarded) - 1; index >= 0 && trustedProxy(address, trustedProxies); index-- {
			candidate, err := netip.ParseAddr(strings.TrimSpace(forwarded[index]))
			if err != nil {
				return claimNetwork(address)
			}
			address = candidate.Unmap()
		}
	}
	return claimNetwork(address)
}

func remoteIP(remoteAddress string) (netip.Addr, bool) {
	value := strings.TrimSpace(remoteAddress)
	if addressPort, err := netip.ParseAddrPort(value); err == nil {
		return addressPort.Addr().Unmap(), true
	}
	host := strings.Trim(value, "[]")
	address, err := netip.ParseAddr(host)
	return address.Unmap(), err == nil
}

func trustedProxy(address netip.Addr, trustedProxies []netip.Prefix) bool {
	for _, prefix := range trustedProxies {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func claimNetwork(address netip.Addr) string {
	address = address.Unmap()
	if address.Is4() {
		return address.String()
	}
	return netip.PrefixFrom(address, 64).Masked().String()
}

func writeClaimRateLimit(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Retry-After", "60")
	writeJSON(w, r, http.StatusTooManyRequests, claimErrorResponse{Error: "claim_unavailable"})
}

func (s *Server) acquireClaimWork() bool {
	select {
	case s.claimWork <- struct{}{}:
		return true
	default:
		return false
	}
}

func (s *Server) releaseClaimWork() {
	<-s.claimWork
}
