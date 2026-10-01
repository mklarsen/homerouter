# Bruno collection

Open this directory as a Bruno collection. The requests use `baseUrl`, `adminUsername`, `adminPassword`, `apiToken`, `proxyUsername`, and `proxyUserPassword` variables. The checked-in local environment provides only the public base URL and non-secret example usernames. Add the passwords and `apiToken` in Bruno's local/secret variables; do not commit real credentials.

The API token is the same `PROXY_API_TOKEN` configured on the proxy and the trusted backend and must be at least 32 characters. It is optional on the server; when unset, use the HTTPS administrator login endpoint and preserve its `homerouter_admin` session cookie.
