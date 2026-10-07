Build this candidate privately with `docker build -f images/runtime/Dockerfile -t claude-cloud-runtime:dev .`.
The build checks Anthropic's signing-key fingerprint, detached manifest signature and pinned architecture checksum.
Native version is 2.1.285. Base images and the Debian package snapshot are pinned; updates require a tested PR.
The build replaces the base image's npm with checksum-pinned 11.21.0 using its bundled dependencies,
offline installation and disabled package lifecycle scripts. Node's major and the vendor binary stay pinned.
The build then replaces npm's bundled brace-expansion/undici with reviewed compatible upstream patch
releases 5.0.11/6.28.1. Each archive is independently checksum-pinned; parent dependency ranges and
installed prerequisites are checked, unsafe archive entries fail, and original license files are retained.
This is a maintained override of the npm bundle, recorded in
`/usr/local/share/claude-runtime/npm-security-overrides.json`; it is not the untouched upstream npm archive.
Update/revalidate these overrides when updating npm and remove them once upstream includes the fixes.
Unresolved distro/runtime findings remain release gates.

No vendor runtime image is published by CI. The operator's Apache-2.0 license does not license the vendor binary.
Redistribution approval and dedicated vendor/network E2E remain release gates. Native registration must use the
per-order credential; never bake an environment key, API key, OAuth login or cloud identity into this image.
