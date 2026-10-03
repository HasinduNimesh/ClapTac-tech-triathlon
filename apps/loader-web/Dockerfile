# Builds the Flutter loader app for the web and serves it under /loader-app/.
FROM ghcr.io/cirruslabs/flutter:stable AS build
WORKDIR /app
COPY pubspec.yaml pubspec.lock ./
RUN flutter pub get
COPY . .
RUN flutter build web --release --base-href /loader-app/ --no-web-resources-cdn

FROM nginx:1.27-alpine
COPY nginx.default.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/build/web /usr/share/nginx/html/loader-app
EXPOSE 80
