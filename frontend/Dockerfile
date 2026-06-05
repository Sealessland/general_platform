FROM node:20-alpine AS build
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

FROM busybox:1.36
COPY --from=build /src/dist /www
EXPOSE 4173
CMD ["httpd", "-f", "-p", "4173", "-h", "/www"]
