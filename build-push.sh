#!/bin/bash
set -euo pipefail

go build
docker build -t gcr.io/cgag-gke/iiif-server .
gcloud docker -- push gcr.io/cgag-gke/iiif-server
