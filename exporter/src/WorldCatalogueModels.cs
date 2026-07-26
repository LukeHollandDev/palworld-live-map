internal sealed record WorldCatalogueLocation(
    string Id,
    string Category,
    string GameId,
    string Name,
    double X,
    double Y,
    double Z)
{
    public string? Detail { get; init; }
    public int? Level { get; init; }
    public string? InstanceId { get; init; }
    public string? StateKey { get; init; }
    public string? ClassName { get; init; }
    public string? IconSource { get; init; }
    public CatalogueReward[]? Rewards { get; init; }
    public string? SourcePackage { get; init; }
    public string? SourceObject { get; init; }
}

internal sealed record CatalogueReward(
    string ItemId,
    string Name,
    int Count,
    string? IconSource,
    string NameSource);

internal sealed record WorldCatalogueDataset(
    int SchemaVersion,
    string Id,
    WorldCatalogueLocation[] Locations);

internal sealed record WorldCatalogueDatasetReference(
    string Id,
    string File,
    int Count,
    string Sha256);

internal sealed record WorldCatalogueScan(
    string PersistentPackage,
    string WorldPartitionPrefix,
    int WorldPartitionPackages,
    WorldCatalogueExclusion[] NpcExclusions);

internal sealed record WorldCatalogueExclusion(
    string Reason,
    int Count);

internal sealed record WorldCatalogueManifest(
    int SchemaVersion,
    string GameVersion,
    string Generator,
    string Decoder,
    SourceFile Mappings,
    SourceFile[] Paks,
    LandmarkSource[] Sources,
    WorldCatalogueScan Scan,
    WorldCatalogueDatasetReference[] Datasets);
