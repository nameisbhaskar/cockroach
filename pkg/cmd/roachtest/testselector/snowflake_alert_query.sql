with ts as (
  select current_date() as t -- this returns the current date
), builds as (
  -- select all the build IDs in the last "forPastDays" days
  select
    ID as run_id,
    min(start_date) as first_run, -- get the first time the test was run
    max(start_date) as last_run, -- -- get the last time the test was run
  from DATAMART_PROD.TEAMCITY.BUILDS, ts
  where
    start_date > dateadd(DAY, -30, ts.t) -- last "forPastDays" days
    and start_date <= ts.t
    and lower(status) = 'success' -- consider only successful builds
    and branch_name = ? -- consider the builds from target branch
    and lower(name) like ? -- name is based on the suite and cloud e.g. '%roachtest nightly - gce%'
  group by 1
), test_stats as (
  -- for all the build ID's select do an inner join on all the tests
  select test_name, -- distinct test names
         max(case when status!='UNKNOWN' then b.last_run end) as last_run,
         MAX_BY(ignore_details, b.last_run) as recent_ignore_details,
  from DATAMART_PROD.TEAMCITY.TESTS t
         -- inner join as explained in the beginning
         inner join builds b on
    b.run_id=t.build_id, ts
  where test_name is not null -- not interested in tests with no names
  group  by 1
)
select test_name, last_run
from test_stats, ts
where last_run < dateadd(DAY, ?, ts.t) -- the test has not been run for last "lastRunOn" days
  and recent_ignore_details = 'test selector'
