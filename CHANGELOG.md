# Changelog

## 0.1.0 (2026-11-08)


### Features

* approve pending claims from the Claude Code question dialog ([#25](https://github.com/nonchan7720/spec-gate/issues/25)) ([f73c3ac](https://github.com/nonchan7720/spec-gate/commit/f73c3ac9bc2c10e1cbde0cdf1db53433bbed5de4))
* auto-approval (HOTL approval), built with spec-gate itself ([#38](https://github.com/nonchan7720/spec-gate/issues/38)) ([f51ad2d](https://github.com/nonchan7720/spec-gate/commit/f51ad2d63d170b2a1b9f28e094366e77a3bf8191))
* check properties on random inputs and metamorphic relations ([#14](https://github.com/nonchan7720/spec-gate/issues/14)) ([ac8ed78](https://github.com/nonchan7720/spec-gate/commit/ac8ed78de1c1c85effee9b38df61d44fc934aa16))
* delegate extract and reverse to independent subagents ([#13](https://github.com/nonchan7720/spec-gate/issues/13)) ([c635295](https://github.com/nonchan7720/spec-gate/commit/c635295372f80b3bdb6c60a0cbe1db7c0bf14bf0))
* detect changes in the package a claim touches ([#16](https://github.com/nonchan7720/spec-gate/issues/16)) ([c76f0fd](https://github.com/nonchan7720/spec-gate/commit/c76f0fd60ef78f8bb1abe47d9404b0c5efe6519b))
* detect example values written into the implementation ([#20](https://github.com/nonchan7720/spec-gate/issues/20)) ([86e1896](https://github.com/nonchan7720/spec-gate/commit/86e1896bad86f7f2d96ff1080a57ad08eebca561))
* dry-run extracted candidates and approve them from an MCP Apps screen ([#29](https://github.com/nonchan7720/spec-gate/issues/29)) ([de53ed3](https://github.com/nonchan7720/spec-gate/commit/de53ed36efeed41ba5ed762d258b320cd7f145fc))
* holdout examples hidden from the agent ([#19](https://github.com/nonchan7720/spec-gate/issues/19)) ([3dc4b57](https://github.com/nonchan7720/spec-gate/commit/3dc4b571e9d79dfb83da5f288a9c2033491729bb))
* JSON output and metrics for CI ([#22](https://github.com/nonchan7720/spec-gate/issues/22)) ([4005612](https://github.com/nonchan7720/spec-gate/commit/4005612a192378c8ee1377ad3ac1b66e14ef116c))
* keep holdout expected values as hashes ([#21](https://github.com/nonchan7720/spec-gate/issues/21)) ([d3180e8](https://github.com/nonchan7720/spec-gate/commit/d3180e8adcc98780ad4917235b87d4d18afa3908))
* record how claims came to pass ([#24](https://github.com/nonchan7720/spec-gate/issues/24)) ([91d79a5](https://github.com/nonchan7720/spec-gate/commit/91d79a53cf4446ff888a6b21718ca54c0cca573a))
* refuse to freeze plans whose verify lines miss mutants ([#18](https://github.com/nonchan7720/spec-gate/issues/18)) ([d2ce344](https://github.com/nonchan7720/spec-gate/commit/d2ce344244f25820412865562cdea62fb78e0e9f))
* reverse-check claims against their spec text ([#9](https://github.com/nonchan7720/spec-gate/issues/9)) ([9d33782](https://github.com/nonchan7720/spec-gate/commit/9d33782ce71e7737a1abee95b47a2bfaa4b8a904))
* review pending claims in a local web page ([#10](https://github.com/nonchan7720/spec-gate/issues/10)) ([5375570](https://github.com/nonchan7720/spec-gate/commit/53755706473137862fbfc3daef74793a7ef03821))
* role reports and ambiguity records ([#23](https://github.com/nonchan7720/spec-gate/issues/23)) ([2ceefff](https://github.com/nonchan7720/spec-gate/commit/2ceefff72b360505f4831ab31fd719ba43bd9a15))
* run the hooks and approval paths on a pinned spec-gate release ([#40](https://github.com/nonchan7720/spec-gate/issues/40)) ([7e2b718](https://github.com/nonchan7720/spec-gate/commit/7e2b71884b622b428862ff7a6db8d45ef906198e))
* spec-gate, a spec-compliance harness with CI and release pipeline ([0074f04](https://github.com/nonchan7720/spec-gate/commit/0074f04cd300a06e30cc8c7b20c4870620cda528))
* suggest the closest heading when a claim's heading is gone ([#15](https://github.com/nonchan7720/spec-gate/issues/15)) ([8bf6211](https://github.com/nonchan7720/spec-gate/commit/8bf62113a7d06f0467bed400120f293b6149a049))


### Bug Fixes

* block the hook when spec-gate cannot judge instead of passing ([#6](https://github.com/nonchan7720/spec-gate/issues/6)) ([fcc1191](https://github.com/nonchan7720/spec-gate/commit/fcc1191c617e50c2200633d57191223de6ffbbfc))
* close gaps that let spec changes or claim typos slip through ([#7](https://github.com/nonchan7720/spec-gate/issues/7)) ([ad0af64](https://github.com/nonchan7720/spec-gate/commit/ad0af64702998bbc861a2fcc8fd83549b63255d6))
* connect the MCP server in cloud sessions, clear the depgraph warnings and draw the README diagrams ([#37](https://github.com/nonchan7720/spec-gate/issues/37)) ([dba57b2](https://github.com/nonchan7720/spec-gate/commit/dba57b2b7e7838e24a35b82871d59ea67162dcd3))
* count paraphrased claims as the same candidate in extract consensus ([#27](https://github.com/nonchan7720/spec-gate/issues/27)) ([491fd68](https://github.com/nonchan7720/spec-gate/commit/491fd68b45bdc0a43dfaf0473799b5894c9e5edb))
* fail structure claims on layer violations that existed before the session ([#5](https://github.com/nonchan7720/spec-gate/issues/5)) ([877c6d3](https://github.com/nonchan7720/spec-gate/commit/877c6d37e7cde66146a837909975ddd8ad51baa8))
* keep every extracted candidate and merge paraphrased ones ([#12](https://github.com/nonchan7720/spec-gate/issues/12)) ([3745a26](https://github.com/nonchan7720/spec-gate/commit/3745a262c9a1da7ca62025bba005d2a1c492b227))
* make the guard hook fail closed without touching the protected settings ([#39](https://github.com/nonchan7720/spec-gate/issues/39)) ([948bc6c](https://github.com/nonchan7720/spec-gate/commit/948bc6ca68675f8657ff99a8fb76a297ee36b5be))
* match the claims guards only on real writes and invocations ([#2](https://github.com/nonchan7720/spec-gate/issues/2)) ([dbdd1d6](https://github.com/nonchan7720/spec-gate/commit/dbdd1d6422ee6986be93328df99d268c865335d6))
* mutate the packages a binding depends on, not just the dispatcher ([#32](https://github.com/nonchan7720/spec-gate/issues/32)) ([ad97c30](https://github.com/nonchan7720/spec-gate/commit/ad97c3017495bd357389b1da00f7faa91dc3f37f))
* point the sample plan's verify lines at the root main package ([#8](https://github.com/nonchan7720/spec-gate/issues/8)) ([5ccb6fb](https://github.com/nonchan7720/spec-gate/commit/5ccb6fb57bb2e9f8b36e5f3da61cb4c25f5fd674))
* re-check claims when a Go package their binding imports changes ([#17](https://github.com/nonchan7720/spec-gate/issues/17)) ([3767432](https://github.com/nonchan7720/spec-gate/commit/3767432e9793b3481e6f9c356885558f997e3761))
* re-check only claims whose own spec or claim file changed ([#30](https://github.com/nonchan7720/spec-gate/issues/30)) ([3669eb9](https://github.com/nonchan7720/spec-gate/commit/3669eb91c732a50fe34101d8570b668b27573a20))
* read the holdout expected output from stdin so it stays out of shell history ([#31](https://github.com/nonchan7720/spec-gate/issues/31)) ([e2cdddc](https://github.com/nonchan7720/spec-gate/commit/e2cdddc1eee2fb8db3f6116d2e7a157cf7e7ea69))
* stop guard false positives and let depgraph give up before the gate kills it ([#33](https://github.com/nonchan7720/spec-gate/issues/33)) ([fd91113](https://github.com/nonchan7720/spec-gate/commit/fd9111379139a3c01a5dcf8104a26962612b0686))


### Documentation

* describe the next harness features in the READMEs ([#26](https://github.com/nonchan7720/spec-gate/issues/26)) ([b239d0a](https://github.com/nonchan7720/spec-gate/commit/b239d0a1c9073734bc095c0045cc90c4132554d6))
* specify the next harness features and add a Go binding dispatcher ([#11](https://github.com/nonchan7720/spec-gate/issues/11)) ([3e0597a](https://github.com/nonchan7720/spec-gate/commit/3e0597a6818d31eb8af0bf9756c73c4ccdaffa86))


### Miscellaneous Chores

* add claim candidates extracted from the harness spec ([#28](https://github.com/nonchan7720/spec-gate/issues/28)) ([33e1c6f](https://github.com/nonchan7720/spec-gate/commit/33e1c6f612bb252d9fe8ad2361bd18a4272cb973))
* **deps:** update dependency vitest to v5 ([#3](https://github.com/nonchan7720/spec-gate/issues/3)) ([9a0cd86](https://github.com/nonchan7720/spec-gate/commit/9a0cd86d90b42b764b8849d1415477d5aa105439))
* **deps:** update jdx/mise-action action to v5.2.5 ([#42](https://github.com/nonchan7720/spec-gate/issues/42)) ([96b1270](https://github.com/nonchan7720/spec-gate/commit/96b1270f192212d3c726591fbf3fd9f7939acaf8))
* pin the hooks to spec-gate v0.0.0-dev.7e2b718 ([#41](https://github.com/nonchan7720/spec-gate/issues/41)) ([0fcfa69](https://github.com/nonchan7720/spec-gate/commit/0fcfa69d0580b83218aea6ba1cb52833a93eaed2))
* set up the cloud session for spec-gate development ([#36](https://github.com/nonchan7720/spec-gate/issues/36)) ([b473211](https://github.com/nonchan7720/spec-gate/commit/b47321108c85842c118a1ebd55d686344e9289b8))
