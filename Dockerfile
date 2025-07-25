FROM golang:1.24 AS build

RUN apt-get update && apt-get install -y curl

# install Node.js 22.x
RUN curl -sL https://deb.nodesource.com/setup_22.x | bash -
RUN apt-get install -y nodejs

RUN mkdir /openpolicyagent
WORKDIR /openpolicyagent/

COPY . .

RUN make all

FROM chainguard/glibc-dynamic:latest
COPY --from=build /openpolicyagent/build/rego-playground /openpolicyagent/
COPY --from=build /openpolicyagent/build/ui/ /openpolicyagent/ui

ENTRYPOINT ["/openpolicyagent/rego-playground"]
