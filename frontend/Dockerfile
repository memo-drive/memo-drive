FROM node:22-alpine AS build

WORKDIR /src
COPY package.json pnpm-lock.yaml ./
RUN npm install -g pnpm@10
RUN corepack enable && pnpm install --frozen-lockfile
COPY . .
RUN pnpm run build

FROM nginx:1.27-alpine

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/dist /usr/share/nginx/html
EXPOSE 80
