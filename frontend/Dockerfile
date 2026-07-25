FROM oven/bun:1 AS build
WORKDIR /src

COPY package.json bun.lock ./
RUN bun install --frozen-lockfile

COPY . .
RUN bun run build

FROM oven/bun:1-slim
WORKDIR /app

COPY --from=build /src/.output ./.output

ENV HOST=0.0.0.0
ENV PORT=3000
EXPOSE 3000
ENTRYPOINT ["bun", ".output/server/index.mjs"]
