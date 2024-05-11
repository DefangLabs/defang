-- Insert 20 random users into the existing "users" table
DO $$
DECLARE
    i INTEGER;
    prefixes TEXT[] := ARRAY['Alpha', 'Bravo', 'Charlie', 'Delta', 'Echo', 'Foxtrot', 'Golf', 'Hotel', 'India', 'Juliet'];
    suffixes TEXT[] := ARRAY['Smith', 'Johnson', 'Williams', 'Brown', 'Jones', 'Garcia', 'Martinez', 'Robinson', 'Clark', 'Rodriguez'];
    random_prefix TEXT;
    random_suffix TEXT;
BEGIN
    FOR i IN 1..20 LOOP
        random_prefix := prefixes[ceil(random() * array_length(prefixes, 1))];
        random_suffix := suffixes[ceil(random() * array_length(suffixes, 1))];
        INSERT INTO "users" ("name") 
        VALUES (
            random_prefix || ' ' || random_suffix
        );
    END LOOP;
END $$;

-- Insert 100 random tasks into the existing "tasks" table
DO $$
DECLARE
    i INTEGER;
    random_user UUID;
    statuses TEXT[] := ARRAY['OPEN', 'IN_PROGRESS', 'COMPLETE'];
    random_status TEXT;
    adjectives TEXT[] := ARRAY['Quick', 'Bright', 'Mysterious', 'Complex', 'Dynamic', 'Strategic', 'Confident', 'Energetic', 'Resolute', 'Agile'];
    nouns TEXT[] := ARRAY['Solution', 'Project', 'Campaign', 'Strategy', 'Initiative', 'Plan', 'Mission', 'Journey', 'Blueprint', 'Concept'];
    random_adjective TEXT;
    random_noun TEXT;
BEGIN
    FOR i IN 1..100 LOOP
        -- Select a random user from the existing "users" table
        SELECT "id" INTO random_user
        FROM "users"
        ORDER BY random()
        LIMIT 1;

        -- Select a random status from the predefined statuses array
        random_status := statuses[ceil(random() * array_length(statuses, 1))];

        -- Generate creative title
        random_adjective := adjectives[ceil(random() * array_length(adjectives, 1))];
        random_noun := nouns[ceil(random() * array_length(nouns, 1))];
        
        -- Insert the random task
        INSERT INTO "tasks" ("title", "description", "status", "assignedTo")
        VALUES (
            random_adjective || ' ' || random_noun || ' ' || i,
            'Detailed description of ' || random_adjective || ' ' || random_noun || ' task.',
            random_status,
            random_user
        );
    END LOOP;
END $$;
