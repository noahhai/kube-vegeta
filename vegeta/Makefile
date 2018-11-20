all: clean loader container push

TAG = latest

loader: loader.go
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -ldflags '-w' -o loader

container: loader
	docker build --pull -t 661058921700.dkr.ecr.us-east-1.amazonaws.com/vegeta-loader:$(TAG) .

push: container
	docker push 661058921700.dkr.ecr.us-east-1.amazonaws.com/vegeta-loader:$(TAG)

clean:
	rm -f loader
	docker image prune --force

auth:
	$$(aws ecr get-login --no-include-email --region us-east-1)