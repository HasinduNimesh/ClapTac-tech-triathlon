# Builds the Flutter loader app for the web and serves it under /loader-app/.
# The Flutter SDK is pinned (the same version the app is analysed and tested with) so a
# new "stable" never changes a production build.
FROM ghcr.io/cirruslabs/flutter:3.41.2 AS build
ARG OIDC_ISSUER=http://localhost:8090
ARG OIDC_CLIENT_ID=waypoint-loader
WORKDIR /app
COPY pubspec.yaml pubspec.lock ./
RUN flutter pub get
COPY . .
RUN flutter build web --release --base-href /loader-app/ --no-web-resources-cdn \
    --dart-define=OIDC_ISSUER=${OIDC_ISSUER} --dart-define=OIDC_CLIENT_ID=${OIDC_CLIENT_ID}

FROM nginx:1.27-alpine
COPY nginx.default.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/build/web /usr/share/nginx/html/loader-app
EXPOSE 80
