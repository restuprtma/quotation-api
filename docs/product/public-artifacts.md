# Public Artifact API

One revision can publish three artifacts:

```text
GET  /p/q/:artifact_token       quotation/pricing
POST /p/q/:artifact_token/sign  quotation signature
GET  /p/b/:artifact_token       blueprint
GET  /p/d/:artifact_token       product/project detail
```

Internal share endpoint:

```text
POST /quotation/v1/quotations/:id/share
GET  /quotation/v1/quotations/:id/artifacts
```

The token is the only public lookup key. The server verifies artifact type and `revoked_at`. Public DTOs exclude tenant ID, creator ID, raw AI response, internal notes, and signed IP.
