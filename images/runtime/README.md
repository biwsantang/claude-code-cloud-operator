Build this candidate privately with `docker build -f images/runtime/Dockerfile -t claude-cloud-runtime:dev .`.
The build checks Anthropic's signing-key fingerprint, detached manifest signature and pinned architecture checksum.
Native version is 2.1.285. Base images and the Debian package snapshot are pinned; updates require a tested PR.

No vendor runtime image is published by CI. The operator's Apache-2.0 license does not license the vendor binary.
Redistribution approval and dedicated vendor/network E2E remain release gates. Native registration must use the
per-order credential; never bake an environment key, API key, OAuth login or cloud identity into this image.
