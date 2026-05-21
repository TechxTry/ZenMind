# ---- Stage 1: Build React ----
FROM node:20-alpine AS builder

WORKDIR /app

COPY package.json package-lock.json* ./
RUN npm install --legacy-peer-deps

COPY . .
ARG APP_VERSION=dev
RUN printf '%s\n' "${APP_VERSION}" > .app-version && npm run build

# ---- Stage 2: Nginx ----
FROM nginx:1.27-alpine

# 移除默认配置
RUN rm /etc/nginx/conf.d/default.conf

# 自定义 nginx 配置（SPA history 路由支持 + API 代理）
COPY nginx.conf /etc/nginx/conf.d/zenmind.conf

# 复制构建产物
COPY --from=builder /app/dist /usr/share/nginx/html

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
