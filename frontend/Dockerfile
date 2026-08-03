# 构建阶段：npm ci（当前无第三方依赖）后执行 npm run build，产出 dist/index.html。
FROM node:20-alpine AS build
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build

# 运行阶段：busybox httpd 静态托管构建产物。
FROM busybox:1.36
COPY --from=build /src/dist /www
EXPOSE 4173
CMD ["httpd", "-f", "-p", "4173", "-h", "/www"]
