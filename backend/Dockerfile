# argument for Go version
ARG GO_VERSION=1.20.5
 
# STAGE 1: building the executable
FROM golang:${GO_VERSION} AS build

WORKDIR /src
COPY ./go.mod ./go.sum ./
RUN go mod download
COPY ./ ./

# Build the executable
RUN CGO_ENABLED=0 go build \
    -installsuffix 'static' \
    -o /backend
 
# STAGE 2: build the container to run
FROM gcr.io/distroless/static AS final

LABEL maintainer="ojgen"
USER nonroot:nonroot
 
# copy configs and compiled app
WORKDIR /app
COPY ./configs/ ./configs/
COPY --from=build --chown=nonroot:nonroot /backend /backend

EXPOSE 8080

ENTRYPOINT ["/backend"]
