DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM quotations
    WHERE char_length(solution_type) > 20
  ) THEN
    RAISE EXCEPTION
      'cannot reduce quotations.solution_type to VARCHAR(20): values longer than 20 characters exist';
  END IF;

  ALTER TABLE quotations
  ALTER COLUMN solution_type TYPE VARCHAR(20);
END
$$;
