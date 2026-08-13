# Save reader patches

`palworld-save-reader-v0.1.0-leaderboards.patch` extends the pinned
`palworld-save-reader` resolve contract with the compact save metrics consumed
by this application. The container build and `make save-reader` both apply it
to the unmodified v0.1.0 source before compiling the external decoder. Both
build paths verify that the tag resolves to commit
`fb88288814f55ceaeb298a1242e96114f30672cc` before applying the patch.

The patched save-reader source remains a separate GPL-3.0-or-later component;
the patch is distributed under the same license. Its source files retain their
copyright and SPDX notices inside the patch.
