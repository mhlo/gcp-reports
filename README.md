# gcp-reports
some useful reports on GCP resources: app-engine apps, backups, etc

This is built mostly around projects which run in multiple environments (per-developer, or standard dev/uat/prod, whatever). It will attempt to list things out according to labels that have set against GCP resources. At this time, those 'things' are projects, and GCS buckets.

The code is currently aimed at App Engine setups, but can be extended to GCE or GKE environments: it uses the standard Google APIs to interrogate.


## Building

Remotely: just do the usual: `go get -u github.com/mhlo/gcp-reports`

### Docker image

You can build a simple Docker image this way:

```
cd $ROOT_DIRECTORY_OF_SOURCE_REPO
docker build -t gcp-reports .
```

Note that this is built from an official Docker hub image, with all the security characteristics that come with that.

## Running

`gcp-reports --help` should get you going. The '-v' option emits more output; be careful if you have very historied project or lots of them. A couple examples:

```
gcp-reports apps foo bar
```
Produces information about all App Engine applications which have a component label of either 'foo' or 'bar'.

```
gcp-reports --env-filter=dev backups
```
Produces information about backups for all the applications which are in the 'dev' environment.

### Docker image

Running the docker image is the same, except for two things:

 * usual Docker stuff: `docker run ...`

 * the Google Cloud application-default credentials are not available within the Docker container.

 The following incantation addresses both of these concerns:

 ```
 docker run --rm -it -v $HOME/.config/gcloud:/root/.config/gcloud gcp-reports --env-filter=dev backups
 ```
#### Google Container Registry

It is sometimes useful to throw this into your GCR. In this case, do the following after building the image locally:

```
# you will probably better versioning that 'latest'!
docker tag gcp-reports:latest gcr.io/YOUR_PROJECT_HERE/gcp-reports
gcloud docker -- push gcr.io/YOUR_PROJECT_HERE/gcp-reports
```

### Labels

These reports are best used against projects and buckets which are _labelled_. These labels categorize resources in ways that are independent of the project-id, or other 1:1 style mapping. It's interesting to filter the query by (at least) two labels:

 * the project environment is described by a key:value pair where the key == `env` by default.
 * the project 'component' is a description of the nature of its capabilities

GCP has methods (found in the 'IAM & Admin' section of any project's conosole) to label a project. GCS buckets have the same capabilities. For GCS, use the command-line:

```
gsutil label ch -l backup:true gs://YOUR_BUCKET_NAME_HERE
```
