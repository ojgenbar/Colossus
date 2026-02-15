# Stage 1: Build Flutter web app
# Pin to amd64 — Flutter images are amd64-only, and the output is
# platform-independent static HTML/JS/CSS anyway.
FROM --platform=linux/amd64 instrumentisto/flutter:3.24 AS build

WORKDIR /app
COPY pubspec.yaml pubspec.lock* ./
RUN flutter pub get
COPY . .
RUN flutter build web --release

# Stage 2: Serve with nginx
FROM nginx:1.25-alpine

# Remove default nginx config
RUN rm /etc/nginx/conf.d/default.conf

# Copy Flutter build output
COPY --from=build /app/build/web /usr/share/nginx/html

# nginx config is mounted via ConfigMap in K8s;
# for standalone Docker usage, copy the local config as fallback
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80
