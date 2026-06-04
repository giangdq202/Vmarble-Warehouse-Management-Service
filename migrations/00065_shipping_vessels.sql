-- +goose Up
CREATE TABLE vessels (
    id                  UUID        PRIMARY KEY,
    name                TEXT        NOT NULL,
    carrier             TEXT,
    voyage_number       TEXT,
    etd                 TIMESTAMPTZ,
    eta                 TIMESTAMPTZ,
    cutoff_date         TIMESTAMPTZ NOT NULL,
    port_of_loading     TEXT,
    port_of_discharge   TEXT,
    created_by          UUID        NOT NULL REFERENCES users(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_vessels_cutoff ON vessels (cutoff_date);

-- One container can only be booked onto one vessel at a time.
CREATE TABLE shipping_bookings (
    id              UUID        PRIMARY KEY,
    vessel_id       UUID        NOT NULL REFERENCES vessels(id),
    container_id    UUID        NOT NULL REFERENCES containers(id),
    booking_ref     TEXT,
    booked_by       UUID        NOT NULL REFERENCES users(id),
    booked_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    note            TEXT,
    CONSTRAINT uq_shipping_booking_container UNIQUE (container_id)
);

CREATE INDEX idx_shipping_bookings_vessel ON shipping_bookings (vessel_id);

-- Denormalize vessel + cutoff onto containers for fast seal-guard (BR-D08)
-- and at-risk dashboard (#296). Set by shipping module when booking changes.
ALTER TABLE containers
    ADD COLUMN vessel_id    UUID        REFERENCES vessels(id),
    ADD COLUMN cutoff_date  TIMESTAMPTZ;

-- +goose Down
ALTER TABLE containers DROP COLUMN IF EXISTS cutoff_date;
ALTER TABLE containers DROP COLUMN IF EXISTS vessel_id;
DROP INDEX  IF EXISTS idx_shipping_bookings_vessel;
DROP TABLE  IF EXISTS shipping_bookings;
DROP INDEX  IF EXISTS idx_vessels_cutoff;
DROP TABLE  IF EXISTS vessels;
