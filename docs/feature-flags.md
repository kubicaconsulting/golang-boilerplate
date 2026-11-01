# Feature flags

Application use cases consume only the evaluation methods they need through a consumer-owned port.
Infrastructure selects local values, the remote LaunchDarkly SDK or the SDK's file data source.
Users listing still evaluates users-list once after authorization; false returns the documented 503.
Feature flags do not grant permissions or bypass authentication.

## Definitions and typed evaluations

Declare flag keys and built-in fallbacks in `internal/platform/featureflags/definitions/flags.go`.
This SDK-free package is the only feature-flag infrastructure package application code may import.
The existing boolean call remains:

```go
s.flags.Bool(ctx, definitions.UsersListKey, actor.Subject, definitions.UsersListFallback)
```

Provider exposes the following methods; ctx is context.Context and keys/subjects are strings:

| Method | Fallback | Result |
|---|---|---|
| Bool | bool | bool |
| String | string | string |
| Enum | string plus allowed string alternatives | (string, error) |
| StringList | []string | []string |
| JSON | json.RawMessage | (json.RawMessage, error) |

Enum uses string-valued LaunchDarkly flags. It rejects a fallback outside its allowed alternatives
as a definition error; unknown evaluated alternatives use the declared fallback. JSON rejects
invalid fallback JSON as a definition error. Other evaluation failures use the appropriate fallback
and emit a structured warning without subjects or flag values. Missing keys and wrong SDK types
use fallbacks too. StringList accepts only JSON arrays whose elements are strings; mixed lists,
objects and null use a fresh fallback copy. An empty array remains a valid empty list.

For future features, add named declarations alongside existing definitions, for example:

```go
const (
    DeliveryModeKey      = "delivery-mode"
    DeliveryModeStandard = "standard"
    DeliveryModeExpress  = "express"
    DeliveryModeFallback = DeliveryModeStandard
    RoutingConfigKey      = "routing-config"
    RoutingConfigFallback = `{"batchSize":10}`
    RegionsKey            = "enabled-regions"
)

// Slices cannot be Go constants; return a fresh value instead of a mutable package global.
func RegionsFallback() []string { return []string{"us"} }
```

A feature's evaluation port can declare Enum, StringList or JSON without SDK types. Use the named
keys/fallbacks rather than repeating scalar literals in use cases. JSON results are owned byte
slices; decode them into an application-owned struct and validate business constraints before use.
On schema/validation failure, use the feature's declared typed fallback. Use JSON with a typed
slice for lists of numbers/objects; StringList is the convenience path for lists of strings.
The definitions above are examples, not additional behavior wired into the users feature.

## Local values and precedence

The default provider is local and needs no remote credentials. `flags.values` and APP_FLAGS_VALUES
accept a JSON-compatible map, preserving existing boolean configuration and adding strings, lists
and objects:

```sh
APP_FLAGS_VALUES='{"users-list":false,"delivery-mode":"express","enabled-regions":["us","eu"],"routing-config":{"batchSize":25}}' make run
```

YAML can express the same nested values directly. Typed configuration defaults reference the
named declarations; base.yaml inherits the users default instead of duplicating it. Configured
values override call-site fallbacks in local mode and supply fallback values for SDK evaluations.
A successful remote/file evaluation wins over configured fallback overrides. Enum/StringList also
validate the evaluated shape/alternatives before returning it.

Snapshots deep-copy nested lists/objects before atomic publication. Failed updates retain the
previous snapshot; returned lists/JSON cannot mutate snapshots or caller fallbacks. Normal Go
configuration callers now use map[string]any instead of map[string]bool. Provider.Update returns
an error for values that cannot be encoded as JSON.

SIGHUP reloads validated local values and log level. Provider selection, SDK credentials and file
paths are static and require restart. Deployed environment changes require restarting the process.
Evaluate once at each application decision point rather than repeatedly within one workflow.

## Remote LaunchDarkly

Set APP_FLAGS_PROVIDER=launchdarkly and APP_FLAGS_SDK_KEY. One SDK client is created per process.
Initialization waits for flags.init_timeout (5 seconds by default); failure logs a warning and
retains explicit fallbacks while the SDK can recover. Streaming updates affect later evaluations.
The authenticated application subject is the user-kind targeting key; no email/name is sent.
The client closes during resource cleanup. Normal tests require no live SDK key.

## Offline JSON files

The file provider uses the official LaunchDarkly ldfiledata data source and the same evaluation
methods. It does not set SDK Offline=true, which would disable evaluation and return fallbacks.
Events are disabled and SDK diagnostics are opted out, so file mode needs no valid remote SDK key.

```sh
APP_FLAGS_PROVIDER=file \
APP_FLAGS_FILES='["configs/flags.local.json"]' \
make run
```

Paths are relative to the process working directory unless absolute. Multiple files are supported;
flag/segment keys must not be duplicated. Missing/invalid files, duplicate keys or initialization
failure reject startup and close any partially created client. The local-values provider remains
available when a simple map is sufficient.

The starter file contains only users-list. More values can use the official simplified format:

```json
{
  "flagValues": {
    "users-list": true,
    "delivery-mode": "express",
    "enabled-regions": ["us", "eu"],
    "routing-config": {"batchSize": 25}
  }
}
```

For equivalent targeting behavior, use the official full `flags` and `segments` snapshot format,
not just evaluated values. The SDK evaluates rules, targeting and standard segments from that data.
Big segments are not supported by file data. Keep a snapshot's prerequisites/segment definitions
with its dependent flags. Matching remote behavior also requires the same contexts and attributes;
the current wrapper supplies only user-kind keys, so rules on other attributes need an explicit
context-contract extension. Files load once at startup: restart after editing or replacing them.
There is no automatic remote-to-file synchronization or file watching in this implementation.

To refresh a snapshot manually, use the SDK endpoint for your account region, for example:

```sh
curl --fail --silent --show-error \
  -H "Authorization: $APP_FLAGS_SDK_KEY" \
  https://sdk.launchdarkly.com/sdk/latest-all > /tmp/launchdarkly-flags.json
# Inspect the result, then place it at your configured flags.files path.
```

Use the EU/federal SDK endpoint for those regions. Do not replace a good snapshot with failed or
partial output; inspect before installing it. Remote snapshots can contain targeting data: mount
account exports as runtime files rather than committing them to the boilerplate. The shipped
starter contains no account data. Containers must mount the selected files read-only and set
APP_FLAGS_FILES to their container paths. The production image includes the safe starter configs,
including flags.local.json; mount account-specific exports instead of baking them into images.

The official [file format and export instructions](https://launchdarkly.com/docs/sdk/features/flags-from-files)
describe SDK snapshots, simplified values and file-data limitations. Live account rules/events
must still be smoke-tested in the deployment environment.
