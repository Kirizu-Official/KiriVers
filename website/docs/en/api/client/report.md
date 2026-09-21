---
title: "Device report"
description: "POST .../clients/report. 200 is ip plus geo only. clients/login is 404."
---

# Device report

`POST /api/v1/projects/{project_ref}/clients/report`

```bash
curl -sS -X POST http://127.0.0.1:8080/api/v1/projects/my-app/clients/report \
  -H 'Content-Type: application/json' \
  -d '{"device_id":"app-stable-id","os":"windows","arch":"x86_64"}'
```

200 body is `ip` plus geo only — no roster wrapper. Leftover `POST .../clients/login` is **404**. With `device_id_policy=none`, sending a device id may be 400 `INVALID_REQUEST`. Schema: [API reference](/en/api/reference/) (`openapi.client.json`; <a href="/en/api/scalar/client" target="_blank" rel="noopener">open Scalar</a>).
