-- 0003_boolean_runtime_columns.sql — align PostgreSQL runtime bool columns.

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
          FROM information_schema.columns
         WHERE table_schema = 'public'
           AND table_name = 'postings'
           AND column_name = 'newcomer'
           AND data_type <> 'boolean'
    ) THEN
        ALTER TABLE postings
            ALTER COLUMN newcomer DROP DEFAULT,
            ALTER COLUMN always_open DROP DEFAULT,
            ALTER COLUMN newcomer TYPE boolean USING newcomer <> 0,
            ALTER COLUMN newcomer SET DEFAULT false,
            ALTER COLUMN always_open TYPE boolean USING always_open <> 0,
            ALTER COLUMN always_open SET DEFAULT false;
    END IF;

    IF EXISTS (
        SELECT 1
          FROM information_schema.columns
         WHERE table_schema = 'public'
           AND table_name = 'ai_extractions'
           AND column_name = 'newcomer'
           AND data_type <> 'boolean'
    ) THEN
        ALTER TABLE ai_extractions
            ALTER COLUMN newcomer TYPE boolean USING newcomer <> 0;
    END IF;
END $$;
