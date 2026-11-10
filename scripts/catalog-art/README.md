# Lossless catalog artwork build tools

The shipped catalog already contains native lossless WebP and needs no conversion to build the game. Runtime dependencies have not changed. These optional Python/Pillow tools are for future reviewed artwork imports and release preparation, not for player startup.

Keep originals outside the source candidate. Freeze a complete current manifest, including all four worlds, then convert into a new nonexistent assets directory. Never point the output at a live source or existing delivery. Pass --history-archive for every previous released source archive whose URLs must survive, or --history-assets for authoritative preserved catalogs. Use --all only for an intentional full conversion.

Example:

python convert_native_webp_catalog.py --source-assets ORIGINAL_ASSETS --dest-assets NEW_SOURCE/internal/catalogart/assets --history-archive PREVIOUS_SOURCE.zip --all
python verify_catalog_history.py --generated-assets NEW_SOURCE/internal/catalogart/assets --history-archive PREVIOUS_SOURCE.zip

Optional --verified-cache CACHE_DIRECTORY checkpoints work outside inputs and outputs. Every reused checkpoint still undergoes original SHA, dimensions, lossless format and exact RGBA comparison; corrupt or mismatched cache entries are rejected. This does not weaken the final checks. Existing released native WebP bytes are copied unchanged to preserve cached redirect destinations.

The converter retains author metadata, original image hashes and generation provenance. Current/retired URLs are preserved via actual-content SHA identities and verified old PNG aliases. Original source files are never overwritten. Previous author identities must remain present in the complete current manifest; intentional author deletion requires a separate compatibility review.

This release used Pillow 13.0.0/libwebp 1.6.0, lossless=True, method=6, exact=True. Tests include alpha including hidden RGB, bad metadata/hash/format, replacement/history retention, incomplete manifests, refusal to overwrite, and corrupted checkpoint rejection.
