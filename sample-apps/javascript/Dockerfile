# SPDX-License-Identifier: GPL-3.0-or-later
FROM node:22.23.5-slim

WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --omit=dev
COPY app.mjs ./

CMD ["node", "app.mjs"]
