all: clean api container push deploy

TAG = latest

# always want build to be called even if we have the output
.PHONY: api
api: api.go
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -ldflags '-w' -o api

container: api
	docker build --pull -t 661058921700.dkr.ecr.us-east-1.amazonaws.com/vegeta-api:$(TAG) .

push: container
	docker push 661058921700.dkr.ecr.us-east-1.amazonaws.com/vegeta-api:$(TAG)

clean:
	#rm -f api
	docker image prune --force

auth:
	$$(aws ecr get-login --no-include-email --region us-east-1)

deploy:
	kubectl delete pod -l app=api

listen:
	name=$$(kubectl get pod -l app=api -o=name); kubectl logs -f $$name