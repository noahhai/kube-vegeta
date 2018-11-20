FROM golang:alpine as builder
RUN apk add -U --no-cache ca-certificates

FROM scratch
ENTRYPOINT []
WORKDIR /
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

ADD api /api
ADD .thy.yml .thy.yml

# TODO : remove this - should fetch fresh, but need CGO_ENABLED=0 on builds for them to work on docker images
ADD thy /thy
# ADD ../scale-up.sh /scaleup.sh
# RUN chmod +777 /scaleup.sh