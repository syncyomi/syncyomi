# Changelog

## [1.1.9](https://github.com/syncyomi/syncyomi/compare/v1.1.8...v1.1.9) (2026-07-10)


### Bug Fixes

* anchor release-please at 1.1.8 point via bootstrap-sha ([#112](https://github.com/syncyomi/syncyomi/issues/112)) ([ab503f3](https://github.com/syncyomi/syncyomi/commit/ab503f31b4fc7f364eb0d4afa3cc45c57e344514))
* bump CI Go toolchain to 1.26.5 for fsnotify v1.10 compatibility ([#105](https://github.com/syncyomi/syncyomi/issues/105)) ([5f88fe6](https://github.com/syncyomi/syncyomi/commit/5f88fe60859a6b2d9640ba216df63499d6b1b532))
* **ci:** wire release-please tags to GoReleaser and bump deprecated actions ([#106](https://github.com/syncyomi/syncyomi/issues/106)) ([3d2cf94](https://github.com/syncyomi/syncyomi/commit/3d2cf948e63469b6dc1f784e592afb0a7108cdbf))
* use last-release-sha so release-please anchors at 1.1.8 ([#114](https://github.com/syncyomi/syncyomi/issues/114)) ([b768e23](https://github.com/syncyomi/syncyomi/commit/b768e237857f7dae2c0317bcbe76883cce40808f))
* **web:** make Vite 8 build work and pin TypeScript to 6.0.3 ([#109](https://github.com/syncyomi/syncyomi/issues/109)) ([1dff8b0](https://github.com/syncyomi/syncyomi/commit/1dff8b0f557c7ab0da264e943c737cc517d8cf71))
* **web:** migrate PostCSS/Tailwind config for v4 ([f2f20a9](https://github.com/syncyomi/syncyomi/commit/f2f20a93293d4eca7fbb574d1ae4c03bb1a37663))


### Other Changes

* add dependabot and release automation ([#85](https://github.com/syncyomi/syncyomi/issues/85)) ([8da9fa8](https://github.com/syncyomi/syncyomi/commit/8da9fa8298c457316f44a27b9c0d90d94a181547))
* run release-please on develop instead of master ([#110](https://github.com/syncyomi/syncyomi/issues/110)) ([fa64710](https://github.com/syncyomi/syncyomi/commit/fa6471033404f5fe6a4eddad5c9ff2e5cbf63f48))


### Dependencies

* bump @fortawesome/free-brands-svg-icons in /web ([#103](https://github.com/syncyomi/syncyomi/issues/103)) ([4ae7124](https://github.com/syncyomi/syncyomi/commit/4ae71240a691ae13d624566a2ec2556372a01668))
* bump actions/download-artifact from 4 to 8 ([#90](https://github.com/syncyomi/syncyomi/issues/90)) ([434f361](https://github.com/syncyomi/syncyomi/commit/434f3613d25501760cab86f5c2b262e09b5e0203))
* bump actions/setup-go from 3 to 6 ([#87](https://github.com/syncyomi/syncyomi/issues/87)) ([3977b9a](https://github.com/syncyomi/syncyomi/commit/3977b9a55559131191eac3ac88a6423b104468cb))
* bump actions/setup-node from 4 to 6 ([#91](https://github.com/syncyomi/syncyomi/issues/91)) ([79c56c6](https://github.com/syncyomi/syncyomi/commit/79c56c6533f862c4973914c9411454b84de3cc62))
* bump alpine from 3.23 to 3.24 ([#89](https://github.com/syncyomi/syncyomi/issues/89)) ([fe41720](https://github.com/syncyomi/syncyomi/commit/fe41720c6910aaee57b6b0f8dba1beb5784e72a6))
* bump docker/setup-buildx-action from 3 to 4 ([#88](https://github.com/syncyomi/syncyomi/issues/88)) ([ff8847e](https://github.com/syncyomi/syncyomi/commit/ff8847ea6305a22334f9ec9bffdb37a20b95439f))
* bump docker/setup-qemu-action from 3 to 4 ([#92](https://github.com/syncyomi/syncyomi/issues/92)) ([713a32d](https://github.com/syncyomi/syncyomi/commit/713a32dae499db887a64966e035b818d50175980))
* bump github.com/fsnotify/fsnotify from 1.6.0 to 1.10.1 ([#93](https://github.com/syncyomi/syncyomi/issues/93)) ([bdf052c](https://github.com/syncyomi/syncyomi/commit/bdf052cbc623992c0e7215258ffac211e8ca96dc))
* bump github.com/google/uuid from 1.3.0 to 1.6.0 ([#97](https://github.com/syncyomi/syncyomi/issues/97)) ([d28e2ba](https://github.com/syncyomi/syncyomi/commit/d28e2ba69070b48b71ac1b40da7194c99bd56625))
* bump github.com/lib/pq from 1.10.7 to 1.12.3 ([#96](https://github.com/syncyomi/syncyomi/issues/96)) ([96398e6](https://github.com/syncyomi/syncyomi/commit/96398e6a1d84fd7a8705f39f7bee6b8307ac0931))
* bump github.com/stretchr/testify from 1.8.1 to 1.11.1 ([#95](https://github.com/syncyomi/syncyomi/issues/95)) ([91ba001](https://github.com/syncyomi/syncyomi/commit/91ba00187d948258ea6e460732aadf3855c18ea7))
* bump golang.org/x/crypto ([#98](https://github.com/syncyomi/syncyomi/issues/98)) ([ff93aeb](https://github.com/syncyomi/syncyomi/commit/ff93aeb7607e1b0ca0b6d4d93dbc835f3e5ba2f0))
* bump node from 20-alpine to 26-alpine ([#94](https://github.com/syncyomi/syncyomi/issues/94)) ([9a66ccb](https://github.com/syncyomi/syncyomi/commit/9a66ccbeb9940a083f8b4f71296ca8c247e546bc))
* bump pinia from 2.3.1 to 3.0.4 in /web ([#99](https://github.com/syncyomi/syncyomi/issues/99)) ([5dfabbd](https://github.com/syncyomi/syncyomi/commit/5dfabbdf99c893e8072f22909acd8b24a22fb747))
* bump tailwindcss from 3.4.19 to 4.3.2 in /web ([2bc3eec](https://github.com/syncyomi/syncyomi/commit/2bc3eec6317c79f8ad7785d282160079a12170bd))
* bump typescript from 5.9.3 to 7.0.2 in /web ([#100](https://github.com/syncyomi/syncyomi/issues/100)) ([de604c0](https://github.com/syncyomi/syncyomi/commit/de604c00f17a59e19b7d78dc3b6d1de7ace75010))
* bump vite from 5.4.21 to 8.1.4 in /web ([#101](https://github.com/syncyomi/syncyomi/issues/101)) ([8d04fee](https://github.com/syncyomi/syncyomi/commit/8d04fee5c9194bb16bcfd8055ca17edcd87aac90))
