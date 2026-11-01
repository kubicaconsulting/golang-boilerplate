# Users reference feature

Create/get/list users demonstrates domain/application/adapter boundaries. `users.NewUser` normalizes
name/email and enforces invariants. Application service checks permissions before persistence or
flag-controlled behavior. Authentication identities remain separate from stored user records.

Repository defines only the operations used here. memory.NewRepository is concurrency-safe and ephemeral;
postgres.NewRepository shares the process pool and maps known uniqueness/no-row failures into domain errors.
The HTTP adapter owns DTOs, schema tags, route metadata and mapping to problem responses.

POST creates one row, which is already atomic. Integration tests demonstrate rollback semantics;
introduce a real application-owned transaction boundary only when a use case needs multiple writes.
Do not add a generic CRUD base class. Extend business rules in domain/use cases first, add a
migration for persisted changes, regenerate OpenAPI, and add tests at the affected boundaries.

Application tests use Testify assertions and Mockery-generated Repository/Flags implementations.
`service_test.go` demonstrates a small test builder, authorization/validation guards, cancellation,
provider outcomes and persistence failures. `mocks_test.go` is generated; update `.mockery.yml`
and run `make mocks` rather than editing it. Real persistence semantics remain integration tests.
