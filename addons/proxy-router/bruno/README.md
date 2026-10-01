# Bruno collection

Open this directory as a Bruno collection. The requests use `baseUrl`, `adminUsername`, `adminPassword`, `apiToken`, `proxyUsername`, `proxyUserPassword`, and `logLevel` variables. The checked-in local environment provides only the public base URL, non-secret example usernames, and default `INFO` log level. Add passwords and `apiToken` in Bruno's local/secret variables; do not commit real credentials.

The API token is the same `PROXY_API_TOKEN` configured on the proxy and the trusted backend and must be at least 32 characters. It is optional on the server; when unset, use the HTTPS administrator login endpoint and preserve its `homerouter_admin` session cookie.

`Disconnect Proxy User` is different: it uses the proxy user's own credentials in the HTTPS JSON body, not the admin Bearer token. It closes every active connection for that username, including connections from other devices using the same account.
