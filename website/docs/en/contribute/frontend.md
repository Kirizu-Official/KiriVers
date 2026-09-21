---
title: "Admin frontend"
description: "yarn dev is the daily loop. yarn build writes dist for embed.go. Production proxies projects."
---

# Admin frontend

Vue 3 + Vite + Vuetify + Pinia + file routes. Dual hey-api clients: `generated/` (admin), `generated-client/` (client). Do not hand-edit generated code. Unwrap list envelopes at the call site (`data?.projects ?? []`).

Daily loop: **`yarn dev`** (:3000). Production: `yarn build` writes `frontend/dist`, embedded by `frontend/embed.go`.

Vite proxy (`frontend/vite.config.mts`): `/api/v1/projects` must precede `/api` ( `:8080` / `:8081` ). Override with `KIRIVERS_CLIENT_API_URL` / `KIRIVERS_API_URL`.

The production process **does** reverse-proxy `/api/v1/projects/**`. Do not document “the admin plane does not forward client paths”.
