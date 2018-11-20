#!/bin/bash

# Create the loadbots
kubectl create -f vegeta-rc.yaml

# Api does aggregator role as well
# # Create the data aggregator
# kubectl create -f aggregator-rc.yaml
# kubectl expose rc aggregator --port=8080

# Create the api
kubectl create -f api-rc.yaml
kubectl expose rc api --port=8080

# kubectl proxy --www=www --port=8001
# http://localhost:8001/static