## ADDED Requirements

### Requirement: UpdateNote with optimistic locking
The system SHALL provide an `UpdateNote` RPC that updates a note's title and content only when the supplied `expected_version` matches the note's current version. The system SHALL increment the version number by 1 upon successful update, refresh `updated_at` to the current time, save an immutable snapshot of the pre-update document to the `note_versions` collection, and invalidate the Redis cache using the delayed double-delete strategy. The system SHALL reject updates with `ErrCodeVersionConflict` when the version does not match.

#### Scenario: Successful update with correct expected_version
- **WHEN** a user calls `UpdateNote` with `note_id="abc"`, `user_id="u1"`, `title="new title"`, `content="new content"`, `expected_version=1` and the note's current version is 1
- **THEN** the note's title and content are updated, `version` becomes 2, `updated_at` is refreshed, a history snapshot (version=1) is written to `note_versions`, and the Redis cache for this note is invalidated (sync + delayed double-delete)

#### Scenario: Update rejected due to version conflict
- **WHEN** a user calls `UpdateNote` with `expected_version=1` but the note's current version is 2
- **THEN** the system returns `ErrCodeVersionConflict` (10005) and no changes are made to the note or history

#### Scenario: Update rejected due to ownership mismatch
- **WHEN** a user calls `UpdateNote` with `user_id="u2"` but the note belongs to `user_id="u1"`
- **THEN** the system returns `ErrCodePermissionDenied` (10003) and no changes are made

#### Scenario: Update rejected for non-existent note
- **WHEN** a user calls `UpdateNote` with a `note_id` that does not exist
- **THEN** the system returns `ErrCodeNoteNotFound` (10002)

#### Scenario: Update rejected for missing required fields
- **WHEN** a user calls `UpdateNote` with empty `note_id`, `user_id`, or `title`
- **THEN** the system returns `ErrCodeInvalidParam` (10001)

### Requirement: ListNoteVersions paginated query
The system SHALL provide a `ListNoteVersions` RPC that returns the version history of a note, sorted by version number in descending order, with pagination support. Only the note owner SHALL be allowed to query history. The default `page_size` SHALL be 20, with a maximum of 50. This RPC SHALL NOT use Redis caching.

#### Scenario: Successful paginated query
- **WHEN** the note owner calls `ListNoteVersions` with `note_id="abc"`, `user_id="u1"`, `page=1`, `page_size=10`
- **THEN** the system returns up to 10 history records sorted by version descending, along with the total count

#### Scenario: Query rejected due to ownership mismatch
- **WHEN** a user calls `ListNoteVersions` with `user_id="u2"` but the note belongs to `user_id="u1"`
- **THEN** the system returns `ErrCodePermissionDenied` (10003)

#### Scenario: Query rejected for non-existent note
- **WHEN** a user calls `ListNoteVersions` with a `note_id` that does not exist
- **THEN** the system returns `ErrCodeNoteNotFound` (10002)

#### Scenario: Default page_size applied
- **WHEN** a user calls `ListNoteVersions` with `page_size=0` or `page_size` not provided
- **THEN** the system applies `page_size=20` as the default

#### Scenario: Page_size capped at maximum
- **WHEN** a user calls `ListNoteVersions` with `page_size=100`
- **THEN** the system caps `page_size` to 50

#### Scenario: Empty history for newly created note
- **WHEN** the note owner queries versions for a note that has never been updated (only created)
- **THEN** the system returns an empty list with `total=0`

### Requirement: RestoreNoteVersion creates new version
The system SHALL provide a `RestoreNoteVersion` RPC that restores a note to a specific historical version by creating a new version with the historical content. The restore SHALL NOT overwrite existing history. The system SHALL validate `expected_version` (optimistic locking), verify ownership, and check that the target version exists in history. After a successful restore, the Redis cache SHALL be invalidated using the delayed double-delete strategy.

#### Scenario: Successful restore
- **WHEN** a user calls `RestoreNoteVersion` with `note_id="abc"`, `user_id="u1"`, `version=2`, `expected_version=5` and the note's current version is 5 and version 2 exists in history
- **THEN** the note's title and content are set to version 2's values, a new version 6 is created, a history snapshot of version 5 is written to `note_versions`, and the Redis cache is invalidated

#### Scenario: Restore rejected due to version conflict
- **WHEN** a user calls `RestoreNoteVersion` with `expected_version=5` but the note's current version is 6
- **THEN** the system returns `ErrCodeVersionConflict` (10005) and no changes are made

#### Scenario: Restore rejected for non-existent version
- **WHEN** a user calls `RestoreNoteVersion` with `version=99` but no such version exists in history
- **THEN** the system returns `ErrCodeVersionNotFound` (10006)

#### Scenario: Restore rejected due to ownership mismatch
- **WHEN** a user calls `RestoreNoteVersion` with `user_id="u2"` but the note belongs to `user_id="u1"`
- **THEN** the system returns `ErrCodePermissionDenied` (10003)

#### Scenario: Restore rejected for non-existent note
- **WHEN** a user calls `RestoreNoteVersion` with a `note_id` that does not exist
- **THEN** the system returns `ErrCodeNoteNotFound` (10002)

### Requirement: Note version field in data model
The `noteDoc` struct and the `Note` Proto message SHALL include a `version` field of type `int32`. Newly created notes SHALL have `version=1`. The `docToPB` function SHALL map the `version` field. Existing notes without a `version` field in cache SHALL deserialize with `version=0` (Go zero value).

#### Scenario: New note created with version 1
- **WHEN** a user calls `CreateNote`
- **THEN** the returned `Note` has `version=1`

#### Scenario: Existing cached note without version field
- **WHEN** a note was cached before the versioning feature was added and `GetNote` reads it from Redis
- **THEN** the `Note` has `version=0` (Go zero value for missing JSON field)

#### Scenario: Note returned with correct version after update
- **WHEN** a note is updated from version 1 to version 2 and then retrieved via `GetNote`
- **THEN** the returned `Note` has `version=2`

### Requirement: Immutable note_versions collection
The system SHALL maintain a `note_versions` MongoDB collection storing immutable history snapshots. Each document SHALL contain `note_id`, `version`, `user_id`, `title`, `content`, and `updated_at`. A compound unique index on `{note_id: 1, version: -1}` SHALL enforce version uniqueness per note. History records SHALL NOT be modified or deleted by any RPC.

#### Scenario: History snapshot written on update
- **WHEN** a note is updated from version 1 to version 2
- **THEN** a document with version=1 and the pre-update content is inserted into `note_versions`

#### Scenario: History snapshot written on restore
- **WHEN** a note at version 5 is restored to version 2, producing version 6
- **THEN** a document with version=5 and the pre-restore content is inserted into `note_versions`

#### Scenario: Unique index prevents duplicate history
- **WHEN** an attempt is made to insert a history record with a `{note_id, version}` combination that already exists
- **THEN** the insert fails with a duplicate key error

### Requirement: New error codes for versioning
The system SHALL define two new error codes: `ErrCodeVersionConflict = 10005` (version mismatch during optimistic lock check) and `ErrCodeVersionNotFound = 10006` (requested historical version does not exist). Existing error codes SHALL remain unchanged.

#### Scenario: Version conflict error returned
- **WHEN** an update or restore operation fails the optimistic lock check
- **THEN** the error code is 10005 with a descriptive message

#### Scenario: Version not found error returned
- **WHEN** a restore operation references a version that does not exist in history
- **THEN** the error code is 10006 with a descriptive message

#### Scenario: Existing error codes unchanged
- **WHEN** existing RPCs (CreateNote, GetNote, ListNotes, DeleteNote) encounter errors
- **THEN** they return the same error codes (10001-10004) as before
