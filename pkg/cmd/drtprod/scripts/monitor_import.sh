#!/bin/bash
# check_and_pause_resume_import_jobs.sh
# This script checks the total number of unavailable ranges.
# If that number is greater than or equal to the upper threshold, it finds all running import jobs and pauses them.
# If the number is lower than the lower threshold, it finds all paused import jobs and resumes them.

if [ "$#" -ne 3 ]; then
  echo "Usage: $0 <CLUSTER> <upper_threshold_for_unavailable_ranges> <lower_threshold_for_unavailable_ranges>"
  exit 1
fi

CLUSTER="$1"
UPPER_THRESHOLD="$2"
LOWER_THRESHOLD="$3"
COCKROACH_CMD="drtprod sql ${CLUSTER}:1 -- -e"  # Adjust connection options as needed.

# Query for total unavailable ranges.
SQL_QUERY_UNAVAILABLE="SELECT sum(value) FROM crdb_internal.node_metrics WHERE name = 'ranges.unavailable';"
result=$(${COCKROACH_CMD} "${SQL_QUERY_UNAVAILABLE}" --format=json 2>/dev/null | grep -v "^Time:" | jq -r '.[0]."sum"')
unavailable_ranges=$(echo "${result}" | xargs)

# Verify that the result is a valid number.
if ! [[ "${unavailable_ranges}" =~ ^[0-9]+$ ]]; then
  echo "$(date) - ERROR: Unable to determine unavailable ranges, got: ${unavailable_ranges}"
  exit 1
fi

echo "$(date) - Current unavailable ranges: ${unavailable_ranges} (Upper Threshold: ${UPPER_THRESHOLD}, Lower Threshold: ${LOWER_THRESHOLD})"

# If unavailable ranges exceed or equal the upper threshold, find and pause any running import jobs.
if [ "${unavailable_ranges}" -ge "${UPPER_THRESHOLD}" ]; then
  echo "$(date) - ALERT: Unavailable ranges (${unavailable_ranges}) are above or equal to the upper threshold (${UPPER_THRESHOLD}). Initiating pause of running import jobs..."

  SQL_QUERY_IMPORT="SELECT job_id FROM [SHOW JOBS] WHERE description ILIKE '%import into%' AND status = 'running';"
  job_ids=$(${COCKROACH_CMD} "${SQL_QUERY_IMPORT}" --format=json 2>/dev/null | grep -v "^Time:" | jq -r '.[].job_id')

  if [ -z "$job_ids" ]; then
    echo "$(date) - No running import jobs found."
  else
    for job_id in $job_ids; do
      # Clean the job_id.
      job_id=$(echo "$job_id" | tr -d '",')
      echo "$(date) - Pausing import job with ID: ${job_id}"
      SQL_PAUSE="PAUSE JOB ${job_id};"
      ${COCKROACH_CMD} "${SQL_PAUSE}" >/dev/null 2>&1
      if [ $? -eq 0 ]; then
         echo "$(date) - Successfully paused job ${job_id}."
      else
         echo "$(date) - ERROR: Failed to pause job ${job_id}."
      fi
    done
  fi

# If unavailable ranges are lower than the lower threshold, find and resume any paused import jobs.
elif [ "${unavailable_ranges}" -lt "${LOWER_THRESHOLD}" ]; then
  echo "$(date) - NOTICE: Unavailable ranges (${unavailable_ranges}) are below the lower threshold (${LOWER_THRESHOLD}). Initiating resume of paused import jobs..."

  SQL_QUERY_IMPORT_PAUSED="SELECT job_id FROM [SHOW JOBS] WHERE description ILIKE '%import into%' AND status = 'paused';"
  paused_job_ids=$(${COCKROACH_CMD} "${SQL_QUERY_IMPORT_PAUSED}" --format=json 2>/dev/null | grep -v "^Time:" | jq -r '.[].job_id')

  if [ -z "$paused_job_ids" ]; then
    echo "$(date) - No paused import jobs found."
  else
    for job_id in $paused_job_ids; do
      job_id=$(echo "$job_id" | tr -d '",')
      echo "$(date) - Resuming import job with ID: ${job_id}"
      SQL_RESUME="RESUME JOB ${job_id};"
      ${COCKROACH_CMD} "${SQL_RESUME}" >/dev/null 2>&1
      if [ $? -eq 0 ]; then
         echo "$(date) - Successfully resumed job ${job_id}."
      else
         echo "$(date) - ERROR: Failed to resume job ${job_id}."
      fi
    done
  fi
else
  echo "$(date) - OK: Unavailable ranges (${unavailable_ranges}) are within thresholds. No action taken."
fi
