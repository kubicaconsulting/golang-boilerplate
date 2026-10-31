# Feature flags

Application use cases consume a boolean-evaluation port. The local provider supplies copied,
atomically published values; unknown keys return the call site's explicit fallback. Users listing
evaluates users-list once after authorization. False returns the documented 503 response.

```sh
APP_FLAGS_VALUES='{"users-list":false}' make run
```

In local/test, edit flags.values in optional YAML and send SIGHUP. In deployments, restart for
changed environment values, or use LaunchDarkly streaming updates. Provider selection requires restart. Feature flags are not authorization and cannot grant missing permissions.

To enable LaunchDarkly, set APP_FLAGS_PROVIDER=launchdarkly and APP_FLAGS_SDK_KEY. One SDK client
is created for the process. Initialization waits for flags.init_timeout; an unready/offline provider
logs a warning and uses explicit local/call-site defaults. It can recover through SDK updates.
Evaluation errors use the same fallback. The application subject is the targeting key; avoid
sending email/name or other user attributes unless the product explicitly needs them.

The client closes during resource cleanup. Normal tests use local flags or the SDK's controlled
test data source; no real SDK key or external network dependency is required. Live account-specific
rules and remote analytics delivery should be smoke-tested in the deployment environment.
