select
  distinct resource_id
from DATAMART_PROD.CLOUD_BILLING.COSTS_COMBINED
where
  account_id='cockroach-ephemeral' and product_name = 'Compute Engine' and
  custom_tags:roachprod::string='true' and custom_tags:usage::string='roachtest' and
  usage_start_date >= dateadd(day, ?, current_date()) and
  custom_tags:spot::string='true' and
  custom_tags:test_run_id::string like 'teamcity-%';
