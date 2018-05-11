load("@io_bazel_rules_go//go:def.bzl", "gazelle", "go_binary", "go_library")

gazelle(
    name = "gazelle",
    args = [
        "-build_file_name",
        "BUILD,BUILD.bazel",
    ],
    command = "fix",
    prefix = "github.com/scele/bazel-remote-tiered-cache",
)

go_library(
    name = "go_default_library",
    srcs = [
        "main.go",
        "s3.go",
    ],
    importpath = "github.com/scele/bazel-remote-tiered-cache",
    visibility = ["//visibility:private"],
    deps = [
        "@com_github_PuerkitoBio_rehttp//:go_default_library",
        "@com_github_aws_aws_sdk_go//aws:go_default_library",
        "@com_github_aws_aws_sdk_go//aws/request:go_default_library",
        "@com_github_aws_aws_sdk_go//aws/session:go_default_library",
        "@com_github_aws_aws_sdk_go//service/s3:go_default_library",
        "@com_github_gregjones_httpcache//:go_default_library",
        "@com_github_gregjones_httpcache//diskcache:go_default_library",
        "@com_github_peterbourgon_diskv//:go_default_library",
    ],
)

go_binary(
    name = "bazel-remote-tiered-cache",
    embed = [":go_default_library"],
    importpath = "github.com/scele/bazel-remote-tiered-cache",
    visibility = ["//visibility:public"],
)
