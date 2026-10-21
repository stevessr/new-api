# Bundled models.dev metadata

The **Models → Sync metadata** wizard can use two sources:

- **Official Repository**: the existing online New API metadata catalog.
- **models.dev (bundled at build)**: a snapshot of the public [models.dev](https://models.dev) model catalog. The backend reads it from its executable; it does not call models.dev at request time.

Both sources use the existing *preview → select models and fields → confirm* process. The source version is checked again when applying changes to avoid stale previews. Models with `sync_official=0` remain protected.

## Refreshing the built-in catalog

Requires Bun 1.4+:

```sh
bun scripts/generate-modelsdev.mjs
```

This downloads `https://models.dev/api.json`, normalizes model names, descriptions, capability tags and vendor names, and writes `controller/modelsdev_snapshot.json`. Docker builds and the Linux/macOS/Windows release workflows run this step automatically **before** compiling Go, so the snapshot is embedded in the binary.

If models.dev is unavailable, packaging **fails explicitly** rather than silently creating an outdated or empty catalog. To build offline while retaining whatever snapshot is in the source tree, set `MODELSDEV_SKIP_FETCH=1` (Docker: `--build-arg MODELSDEV_SKIP_FETCH=1`). The checked-in placeholder is empty, so an offline build without a previously generated snapshot will show a descriptive error when models.dev is selected; the online Official Repository option remains usable.

Tests:

```sh
bun test scripts/generate-modelsdev.test.mjs
go test ./controller ./model
```

## Field policy

This source imports **descriptive metadata only**: model ID, description (including context/output limits when published), capability tags and vendor. It intentionally does not import provider-specific pricing, credentials, model routing, or protocol endpoint assumptions.

For existing model records, the models.dev sync wizard only offers **Description**, **Tags** and **Vendor** changes. The local icon, endpoints, matching rule, visibility, and model pricing are preserved. New records receive the normal catalog defaults and can be edited before they are publicly advertised or bound to channels.

models.dev provider entries sometimes share the same model ID. The generator deterministically prefers a first-party provider, and de-duplicates vendor names. The bundled data is a snapshot as of build time, not a live model capability guarantee.
