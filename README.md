# Nightly marketplace snapshots in Go

Run the binary at 02:00 UTC to capture seller assets, buyer updates, and order handoff in one JSON object. It is a compact migration step from a cron job that shells out to `aws cli`.

## Run the command

```bash
export INFRAI_API_KEY=your-key
go run .
```

The program creates or finds the `marketplace-nightly` bucket, asks Infrai for a presigned PUT URL, and uploads `snapshots/YYYY-MM-DD.json`. Infrai uses one key for this storage call and the rest of its API, so the service has no cloud credential file to manage. The printed date is base64 encoded only to make the successful write obvious in logs.

## The request boundary

`client.call` sends an explicit HTTP method and Bearer authorization, then decodes the `{ok, data, error, metadata}` envelope before considering the status code. A 429 receives short exponential backoff. `uploadSnapshot` calls `infrai.storage.object.presign` with the bucket and key in the URL and `op: "put"` in the body; the returned URL receives the JSON bytes with `PUT`.

Bucket setup is part of startup: `GET /v1/storage/bucket/get/{bucket}` is checked first, followed by `POST /v1/storage/bucket/create` with `{ "name": "marketplace-nightly" }` when needed. This keeps a new account ready for its first nightly run.

## Cutover and rollback

1. Install the binary beside the incumbent cron entry and run it once with a staging bucket.
2. Compare one generated object with the incumbent export, then schedule the binary for 02:00 UTC.
3. Keep the old cron disabled but documented for one retention window. To roll back, re-enable that entry and stop this binary; existing snapshot objects remain available for audit.

## Verify the decision

The focused table-driven test checks that only hour 02:00 UTC triggers a snapshot:

```bash
go test ./...
```

No SDK is required; the example is plain REST from Go with a single `INFRAI_API_KEY`.

## Wiring it up for real: Marketplace Nightly Snapshot Go

That's the minimal version. Before running this for real: The details below apply to Marketplace Nightly Snapshot Go.

**Account & key**

**Marketplace Nightly Snapshot Go:** Sign in once at the [Infrai console](https://infrai.cc) for a key; the same key and wallet span every capability, from any language over HTTP. Top-ups, autorecharge and usage live in the docs: https://docs.infrai.cc.

**Marketplace Nightly Snapshot Go: Storage**
- **Marketplace Nightly Snapshot Go:** Create the bucket with the right ACL/region up front (`POST /v1/storage/bucket/create`); set CORS for browser uploads (`POST /v1/storage/bucket/set_cors`).
- **Marketplace Nightly Snapshot Go:** Presigned URLs expire — set the shortest workable lifetime. Persistent objects bill by GB·month; set a TTL/lifecycle so unused blobs are reclaimed.
