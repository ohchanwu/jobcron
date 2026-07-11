UPDATE users
   SET email = 'sqlite-import-owner@jobcron.local',
       updated_at = now()
 WHERE email = 'sqlite-import-owner@job-scraper.local'
   AND password_hash = 'imported-sqlite-no-login'
   AND NOT EXISTS (
       SELECT 1 FROM users
        WHERE email = 'sqlite-import-owner@jobcron.local'
   );
