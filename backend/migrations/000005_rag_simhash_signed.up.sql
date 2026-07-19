ALTER TABLE rag_source_versions
    ADD COLUMN sim_hash64_signed BIGINT NULL AFTER normalized_hash;

UPDATE rag_source_versions
SET sim_hash64_signed = CAST(sim_hash64 AS SIGNED);

DROP INDEX idx_rag_source_versions_sim_hash64 ON rag_source_versions;

ALTER TABLE rag_source_versions
    DROP COLUMN sim_hash64,
    CHANGE COLUMN sim_hash64_signed sim_hash64 BIGINT NOT NULL DEFAULT 0,
    ADD INDEX idx_rag_source_versions_sim_hash64 (sim_hash64);
