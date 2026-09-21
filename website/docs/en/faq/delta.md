---
title: "Download and delta"
description: "Magic cross-decode, DELTA_ALGO_UNSUPPORTED, Job failure, pack fallback to full."
---

# Download and delta

Do not cross-decode: `KVDIFFHP1`, `HDIFF13&`, `BSDIFF40`, VCDIFF `D6 C3 C4`. Unknown magic → full package. Unknown admin algo → `DELTA_ALGO_UNSUPPORTED`. Failed delta Jobs: bell / `JOB_FAILED`.

Multi-file pack oversize is **200** `full_package`, not 400. Client pack has **no** admin `job_id`. `local_sha256` exists only on diff. Downloads are `/packages/{sha256}`, not `/artifacts/{id}/{filename}`.
