CGO_ENABLED=0 GOOS=linux  go build -a -installsuffix cgo -o main .


# if on unix subsystem:
export DOCKER_HOST=localhost:2375

docker build -t local/gcd -f .\Dockerfile.gcd .

docker push 661058921700.dkr.ecr.us-east-1.amazonaws.com/test-grpc-gcd:latest

docker tag local/gcd:latest 661058921700.dkr.ecr.us-east-1.amazonaws.com/test-grpc-gcd:latest