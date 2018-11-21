all: clean loader container push deploy

TAG = latest

# always want build to be called even if we have the output
.PHONY: loader
loader: loader.go
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -ldflags '-w' -o loader

container: loader
	docker build --pull -t 661058921700.dkr.ecr.us-east-1.amazonaws.com/vegeta-loader:$(TAG) .

push: container
	docker push 661058921700.dkr.ecr.us-east-1.amazonaws.com/vegeta-loader:$(TAG)

clean:
	#rm -f loader
	docker image prune --force

auth:
	$$(aws ecr get-login --no-include-email --region us-east-1)

deploy:
	kubectl delete pod -l run=vegeta

listen:
	name=$$(kubectl get pod -l run=vegeta -o=name); kubectl logs -f $$name