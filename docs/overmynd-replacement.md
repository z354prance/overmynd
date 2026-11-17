# Overmynd replacement baseline

Overmynd now continues the Undermynd source snapshot at 0f46f7b, preserving
Overmynd repository history. This is the baseline for the forthcoming roadmap.

The project name, module, executable and publishing package are Overmynd.
Existing Undermynd configuration, keys, working paths, download-client login and
UNDERMYND_* environment variables remain compatible. OVERMYND_* aliases take
precedence. Preserve the mounts and environment of the current Undermynd container.

The original dashboard-only Overmynd database is not a configuration for this
application. See the README before switching container images.
