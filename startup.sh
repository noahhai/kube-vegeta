#!/bin/bash

# Create the loadbots
kubectl create -f vegeta-rc.yaml

# Create the data aggregator
kubectl create -f aggregator-rc.yaml
kubectl expose rc aggregator --port=8080

# Create the api
kubectl create -f api-rc.yaml
kubectl expose rc aggregator --port=8080

# kubectl proxy --wwww=www --port=8001
# http://localhost:8001/static