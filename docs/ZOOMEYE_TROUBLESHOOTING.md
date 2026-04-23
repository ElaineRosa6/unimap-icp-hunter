# ZoomEye API Troubleshooting

## Issue: 402 "credits_insufficent"
You are encountering a `402 credits_insufficent` error despite having `remain_free_quota: 3000`.

## Research Findings

1.  **"Registered User" API Access**: 
    -   Yes, "Registered Users" (Free Plan) have API access, usually limited to 1,000 or 10,000 queries per month depending on the exact signup date and policy.
    
2.  **Specific Restrictions (The Likely Cause)**:
    -   **Mobile Verification**: ZoomEye strictly requires accounts to be **mobile verified** (bind a phone number) to activate the free API quota. 
    -   Without mobile verification, the system displays the quota but blocks API calls with `402` or `403`.
    -   **Action**: Please log in to ZoomEye.org, go to your Profile, and ensure your mobile number is verified.

3.  **Header Correctness**:
    -   The header `API-KEY: <uuid>` is **correct** for the ZoomEye V3 API (which is what standard endpoints like `/host/search` use).
    -   (Legacy V2 API used JWT tokens, but you are using the correct V3 format).

4.  **Required Parameters**:
    -   There are no hidden parameters required to "consume" the free quota. It is consumed automatically if the account status is valid.
    -   Pagination parameters (`page`, `pageSize`) are handled correctly by the adapter.

## Debugging Tool
I have created a debug tool to help isolate the issue: `cmd/debug-zoomeye/main.go`.

Run it with your API Key:
```bash
go run cmd/debug-zoomeye/main.go -apikey "YOUR_API_KEY"
```

This will check:
1.  `/resources-info` (User Info & Quota)
2.  `/host/search` (Host Search - Port/Service)
3.  `/web/search` (Web Search - Website/Component)

## Code Updates
I have updated `internal/adapter/zoomeye.go` to provide a more descriptive error message when a 402 occurs, suggesting account verification.
