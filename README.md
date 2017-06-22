# gcp-reports
some useful reports on GCP resources: app-engine apps, backups, etc

This is built mostly around projects which run in multiple environments (per-developer, or standard dev/uat/prod, whatever). It will attempt to list things out according to labels that have set against GCP resources. At this time, those are projects, and GCS buckets.

The code is currently aimed at App Engine setups, but can be extended to GCE or GKE environments: it uses the standard Google APIs to interrogate.

## Building

Remotely: just do the usual: `go get -u github.com/mhlo/gcp-reports`

## Running

`gcp-reports --help` should get you going. The '-v' option emits more output; be careful if you have very historied project or lots of them.




