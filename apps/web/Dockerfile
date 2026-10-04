FROM node:20-alpine AS build
WORKDIR /app
ARG VITE_OIDC_ISSUER=http://localhost:8090
ARG VITE_OIDC_CLIENT_ID=waypoint-web
ARG VITE_OIDC_REDIRECT_URI=http://localhost/auth/callback
ARG VITE_OIDC_AUDIENCE=waypoint-api
ARG VITE_API_BASE_URL=/api/v1
ENV VITE_OIDC_ISSUER=$VITE_OIDC_ISSUER
ENV VITE_OIDC_CLIENT_ID=$VITE_OIDC_CLIENT_ID
ENV VITE_OIDC_REDIRECT_URI=$VITE_OIDC_REDIRECT_URI
ENV VITE_OIDC_AUDIENCE=$VITE_OIDC_AUDIENCE
ENV VITE_API_BASE_URL=$VITE_API_BASE_URL
COPY package.json package-lock.json* ./
RUN npm install
COPY . .
RUN npm run build

FROM nginx:1.27-alpine
COPY nginx.default.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80
