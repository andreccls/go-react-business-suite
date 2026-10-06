-- Money is an integer number of cents. Times are timestamptz (stored in UTC).

CREATE TABLE services (
    id           uuid        PRIMARY KEY,
    name         text        NOT NULL,
    description  text        NOT NULL DEFAULT '',
    duration_min integer     NOT NULL CHECK (duration_min BETWEEN 5 AND 480),
    price_cents  integer     NOT NULL CHECK (price_cents >= 0),
    active       boolean     NOT NULL DEFAULT true,
    created_at   timestamptz NOT NULL,
    updated_at   timestamptz NOT NULL
);

CREATE TABLE customers (
    id         uuid        PRIMARY KEY,
    name       text        NOT NULL,
    email      text        NOT NULL,
    phone      text        NOT NULL DEFAULT '',
    notes      text        NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT customers_email_key UNIQUE (email)
);

-- "New customers in a period" (dashboard).
CREATE INDEX customers_created_at_idx ON customers (created_at);

CREATE TABLE appointments (
    id            uuid        PRIMARY KEY,
    customer_id   uuid        NOT NULL,
    service_id    uuid        NOT NULL,
    -- Snapshot of the service at booking time (ADR 0003): later edits never rewrite history.
    service_name  text        NOT NULL,
    duration_min  integer     NOT NULL CHECK (duration_min > 0),
    price_cents   integer     NOT NULL CHECK (price_cents >= 0),
    starts_at     timestamptz NOT NULL,
    ends_at       timestamptz NOT NULL,
    status        text        NOT NULL CHECK (status IN ('scheduled', 'completed', 'cancelled', 'no_show')),
    notes         text        NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL,
    updated_at    timestamptz NOT NULL,
    CONSTRAINT appointments_customer_fk FOREIGN KEY (customer_id) REFERENCES customers (id) ON DELETE RESTRICT,
    CONSTRAINT appointments_service_fk  FOREIGN KEY (service_id)  REFERENCES services (id)  ON DELETE RESTRICT,
    CONSTRAINT appointments_ends_after_start CHECK (ends_at > starts_at),
    -- The single agenda cannot hold two appointments that overlap in time (ADR 0002).
    -- Half-open range: back-to-back appointments are fine. Cancelled and no-show
    -- appointments do not occupy the agenda. The database enforces it atomically, so
    -- concurrent requests for one slot cannot both succeed.
    CONSTRAINT appointments_no_overlap EXCLUDE USING gist (tstzrange(starts_at, ends_at, '[)') WITH &&)
        WHERE (status IN ('scheduled', 'completed'))
);

-- Dashboard aggregations and the calendar: a range scan on starts_at that can be answered
-- from the index alone (index-only scan) for the status/price aggregations (ADR 0004).
CREATE INDEX appointments_starts_at_idx ON appointments (starts_at) INCLUDE (status, price_cents, service_id);
-- "Upcoming appointments": scheduled ones only, soonest first.
CREATE INDEX appointments_scheduled_idx ON appointments (starts_at) WHERE status = 'scheduled';
-- Per-customer history, and the foreign-key checks on delete.
CREATE INDEX appointments_customer_idx ON appointments (customer_id, starts_at);
CREATE INDEX appointments_service_idx ON appointments (service_id);
