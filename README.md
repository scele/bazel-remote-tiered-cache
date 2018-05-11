# Bazel remote tiered cache

A bazel remote cache server that implements a two-tier cache.
The first tier is a read-only local filesystem cache.
The second tier is a read-write S3 cache.

Features:
* Support for dropping write requests to expose both tiers as read-only
* Support for retrying the S3 requests on failure
* If the request misses in the S3 cache, it is still cached in the filesystem
  cache, but the S3 cache will be re-tried after a configurable time period.

This project combines ideas from the following projects:

* [notnoopci/bazel-remote-proxy](https://github.com/notnoopci/bazel-remote-proxy)
* [buchgr/bazel-remote](https://github.com/buchgr/bazel-remote)
* [gregjones/httpcache](https://github.com/gregjones/httpcache)
