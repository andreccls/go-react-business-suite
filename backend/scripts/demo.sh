#!/bin/sh
# End-to-end tour of the running API with curl, ending with a dashboard whose numbers are
# CHECKED against a manual computation. Needs curl, jq and docker (for one step, see below).
#   make up && make demo        (API_PORT / ADMIN_* override the defaults)
#
# Needs an EMPTY agenda (a fresh `make up`): the expected KPIs are computed from the rows
# this script creates. Re-run after `make clean && make up`. It ends by exhausting the
# per-IP login rate limit (10/min), so wait a minute before another run.
#
# About the one non-API step: the API refuses bookings in the past BY DESIGN, so the
# script inserts a short appointment HISTORY (last 10 days) straight into PostgreSQL with
# psql (inside the compose postgres container), through the same table and constraints.
set -eu
BODYF=$(mktemp); HDRF=$(mktemp); trap 'rm -f "$BODYF" "$HDRF"' EXIT
API="${API_URL:-http://localhost:${API_PORT:-8095}}"
ADMIN_EMAIL="${ADMIN_EMAIL:-admin@example.com}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-dev-only-admin-password}"
COMPOSE_FILE_PATH="$(cd "$(dirname "$0")/../.." && pwd)/docker-compose.yml"
S=$(date +%s)
J='Content-Type: application/json'

# Portable date arithmetic (GNU date on Linux, BSD date on macOS).
day() { date -u -d "$1 days" +%Y-%m-%d 2>/dev/null || date -u -v"$(printf '%+d' "$1")"d +%Y-%m-%d; }
secs() { date -u -d "+$1 seconds" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -v+"$1"S +%Y-%m-%dT%H:%M:%SZ; }

call() { # call <label> <curl args...>   -> prints "<status> <label>", body in $BODYF
  label=$1; shift
  code=$(curl -s -o "$BODYF" -D "$HDRF" -w '%{http_code}' "$@")
  printf '%-4s %s\n' "$code" "$label"
}
body() { jq -c . "$BODYF" 2>/dev/null || cat "$BODYF"; }
expect() { # expect <status> <label> <curl args...>: fails the demo on a different status
  want=$1; shift; call "$@"
  [ "$code" = "$want" ] || { echo "   !! expected $want, got $code"; body; exit 1; }
}
auth() { echo "Authorization: Bearer $1"; }
post() { # post <token> <path> <json>
  curl -s -o "$BODYF" -D "$HDRF" -w '%{http_code}' -H "$(auth "$1")" -H "$J" "$API$2" -d "$3"
}
check() { # check <label> <expected> <got>
  if [ "$2" = "$3" ]; then printf '   ok   %-34s expected %-8s got %s\n' "$1" "$2" "$3"
  else printf '   FAIL %-34s expected %-8s got %s\n' "$1" "$2" "$3"; FAILED=1; fi
}
FAILED=0

echo "== docs & health"
expect 200 "GET /docs/ (Swagger UI)" "$API/docs/"
expect 200 "GET /openapi.json" "$API/openapi.json"
printf '     %s operations in the contract\n' "$(jq '[.paths[] | keys[] | select(. != "parameters")] | length' "$BODYF")"
expect 200 "GET /healthz" "$API/healthz"
expect 200 "GET /readyz" "$API/readyz"

echo "== auth"
expect 401 "protected route without token (401 missing_token)" "$API/v1/services"; body
expect 401 "login with a wrong password (401 invalid_credentials)" -H "$J" "$API/v1/auth/login" -d "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"wrong-password\"}"
expect 200 "login admin" -H "$J" "$API/v1/auth/login" -d "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\"}"
ADMIN=$(jq -r .access_token "$BODYF"); REFRESH=$(jq -r .refresh_token "$BODYF")
echo "     expires_in=$(jq .expires_in "$BODYF")s token_type=$(jq -r .token_type "$BODYF")"
expect 200 "GET /v1/auth/me" -H "$(auth "$ADMIN")" "$API/v1/auth/me"; body
expect 200 "refresh (rotation)" -H "$J" "$API/v1/auth/refresh" -d "{\"refresh_token\":\"$REFRESH\"}"
expect 401 "reuse of the OLD refresh token (401, whole family revoked)" -H "$J" "$API/v1/auth/refresh" -d "{\"refresh_token\":\"$REFRESH\"}"
expect 201 "admin creates a staff user" -H "$(auth "$ADMIN")" -H "$J" "$API/v1/users" -d "{\"email\":\"staff$S@example.com\",\"password\":\"staff-password-1\",\"role\":\"staff\"}"; body
expect 200 "login staff" -H "$J" "$API/v1/auth/login" -d "{\"email\":\"staff$S@example.com\",\"password\":\"staff-password-1\"}"
STAFF=$(jq -r .access_token "$BODYF")
expect 403 "staff cannot create users (403 forbidden)" -H "$(auth "$STAFF")" -H "$J" "$API/v1/users" -d '{"email":"x@example.com","password":"long-enough-pass","role":"staff"}'

echo "== the agenda must be empty (this demo checks exact numbers)"
expect 200 "GET /v1/appointments" -H "$(auth "$STAFF")" "$API/v1/appointments?page_size=1"
if [ "$(jq .total "$BODYF")" != "0" ]; then
  echo "   The agenda already has appointments. Run: make clean && make up && make demo"; exit 2
fi

echo "== catalog (price in cents) and customers"
svc() { # svc <name> <minutes> <cents> [active]
  post "$STAFF" /v1/services "{\"name\":\"$1\",\"duration_min\":$2,\"price_cents\":$3,\"active\":${4:-true}}" >/dev/null; jq -r .id "$BODYF"
}
CUT=$(svc "Haircut" 30 5000); COLOR=$(svc "Hair color" 90 20000); MASSAGE=$(svc "Deep tissue massage" 60 12000)
EXPRESS=$(svc "Express check" 5 3000); OLD=$(svc "Retired service" 30 1000 false)
expect 422 "invalid service (422 validation_failed)" -H "$(auth "$STAFF")" -H "$J" "$API/v1/services" -d '{"name":"x","duration_min":1,"price_cents":-1}'; body
expect 200 "list active services" -H "$(auth "$STAFF")" "$API/v1/services?active=true"
printf '     %s active services: %s\n' "$(jq .total "$BODYF")" "$(jq -r '[.data[].name] | join(", ")' "$BODYF")"
cust() { post "$STAFF" /v1/customers "{\"name\":\"$1\",\"email\":\"$2$S@example.com\",\"phone\":\"(31) 99999-000$3\"}" >/dev/null; jq -r .id "$BODYF"; }
ANA=$(cust "Ana Souza" ana 1); BRUNO=$(cust "Bruno Lima" bruno 2); CARLA=$(cust "Carla Dias" carla 3); DAVI=$(cust "Davi Rocha" davi 4)
expect 409 "duplicate e-mail (409 email_taken)" -H "$(auth "$STAFF")" -H "$J" "$API/v1/customers" -d "{\"name\":\"Ana Again\",\"email\":\"ANA$S@example.com\"}"
expect 200 "search customers q=souza" -H "$(auth "$STAFF")" "$API/v1/customers?q=souza"
printf '     found: %s\n' "$(jq -r '[.data[].name] | join(", ")' "$BODYF")"

echo "== booking rules (future agenda)"
D2=$(day 2); D3=$(day 3)
book() { post "$STAFF" /v1/appointments "{\"customer_id\":\"$1\",\"service_id\":\"$2\",\"starts_at\":\"$3\"}"; }
code=$(book "$ANA" "$CUT" "${D2}T14:00:00Z"); printf '%-4s book Haircut %sT14:00Z (30 min)\n' "$code" "$D2"
A1=$(jq -r .id "$BODYF"); echo "     ends_at=$(jq -r .ends_at "$BODYF") price_cents=$(jq .price_cents "$BODYF") status=$(jq -r .status "$BODYF")"
code=$(book "$BRUNO" "$CUT" "${D2}T14:15:00Z"); printf '%-4s overlapping booking at 14:15 (409 slot_unavailable)  ' "$code"; body
[ "$code" = 409 ] || exit 1
code=$(book "$BRUNO" "$CUT" "${D2}T14:30:00Z"); printf '%-4s back-to-back at 14:30 is fine\n' "$code"; [ "$code" = 201 ] || exit 1
code=$(book "$ANA" "$CUT" "$(day -1)T14:00:00Z"); printf '%-4s booking in the past (422)  ' "$code"; body; [ "$code" = 422 ] || exit 1
code=$(book "$ANA" "$OLD" "${D3}T14:00:00Z"); printf '%-4s inactive service (422 service_inactive)  ' "$code"; body; [ "$code" = 422 ] || exit 1
code=$(book "$CARLA" "$COLOR" "${D3}T14:00:00Z"); printf '%-4s book Hair color %sT14:00Z (90 min)\n' "$code" "$D3"; A3=$(jq -r .id "$BODYF")
code=$(book "$DAVI" "$MASSAGE" "${D3}T14:00:00Z"); printf '%-4s same slot, other customer (409)\n' "$code"; [ "$code" = 409 ] || exit 1

echo "== snapshot: editing the service does not rewrite history"
expect 200 "PATCH Haircut price 5000 -> 9900" -H "$(auth "$STAFF")" -H "$J" -X PATCH "$API/v1/services/$CUT" -d '{"price_cents":9900}'
expect 200 "GET the appointment booked before" -H "$(auth "$STAFF")" "$API/v1/appointments/$A1"
check "frozen price_cents" 5000 "$(jq .price_cents "$BODYF")"
expect 200 "PATCH Haircut price back to 5000" -H "$(auth "$STAFF")" -H "$J" -X PATCH "$API/v1/services/$CUT" -d '{"price_cents":5000}'

echo "== status transitions"
expect 409 "complete a FUTURE appointment (409 not_started)" -H "$(auth "$STAFF")" -H "$J" -X PATCH "$API/v1/appointments/$A1/status" -d '{"status":"completed"}'; body
expect 200 "cancel the Hair color appointment" -H "$(auth "$STAFF")" -H "$J" -X PATCH "$API/v1/appointments/$A3/status" -d '{"status":"cancelled"}'
expect 409 "cancelled is terminal (409 invalid_transition)" -H "$(auth "$STAFF")" -H "$J" -X PATCH "$API/v1/appointments/$A3/status" -d '{"status":"completed"}'
code=$(book "$DAVI" "$MASSAGE" "${D3}T14:00:00Z"); printf '%-4s the cancelled slot is free again: Massage booked\n' "$code"; [ "$code" = 201 ] || exit 1
# A live one: a 5-minute service that starts in a few seconds, completed once it started.
code=$(book "$ANA" "$EXPRESS" "$(secs 4)"); printf '%-4s book Express check starting in ~4 s\n' "$code"; A5=$(jq -r .id "$BODYF")
expect 409 "complete it before it starts (409 not_started)" -H "$(auth "$STAFF")" -H "$J" -X PATCH "$API/v1/appointments/$A5/status" -d '{"status":"completed"}'
sleep 5
expect 200 "complete it after it started" -H "$(auth "$STAFF")" -H "$J" -X PATCH "$API/v1/appointments/$A5/status" -d '{"status":"completed"}'

echo "== history for the dashboard (psql: the API refuses past bookings by design)"
# offset-days  hour(UTC)  service  customer  status         -> same table drives the expectations below
HISTORY="-1 15 CUT ANA completed
-1 16 COLOR BRUNO completed
-2 15 MASSAGE CARLA completed
-2 17 CUT DAVI cancelled
-3 15 COLOR ANA completed
-3 18 CUT BRUNO no_show
-4 15 CUT CARLA completed
-5 15 MASSAGE DAVI completed
-5 17 MASSAGE ANA no_show
-6 15 CUT BRUNO completed
-7 15 COLOR CARLA cancelled
-8 15 CUT DAVI completed
-9 15 MASSAGE ANA completed
-10 15 COLOR BRUNO completed"
price() { case $1 in CUT) echo 5000;; COLOR) echo 20000;; MASSAGE) echo 12000;; esac; }
mins() { case $1 in CUT) echo 30;; COLOR) echo 90;; MASSAGE) echo 60;; esac; }
sid() { case $1 in CUT) echo "$CUT";; COLOR) echo "$COLOR";; MASSAGE) echo "$MASSAGE";; esac; }
sname() { case $1 in CUT) echo "Haircut";; COLOR) echo "Hair color";; MASSAGE) echo "Deep tissue massage";; esac; }
cid() { case $1 in ANA) echo "$ANA";; BRUNO) echo "$BRUNO";; CARLA) echo "$CARLA";; DAVI) echo "$DAVI";; esac; }
SQL=""
while read -r off hour sv cu st; do
  [ -n "$off" ] || continue
  ts="$(day "$off")T$(printf '%02d' "$hour"):00:00Z"
  SQL="$SQL INSERT INTO appointments (id, customer_id, service_id, service_name, duration_min, price_cents, starts_at, ends_at, status, created_at, updated_at)
   VALUES (gen_random_uuid(), '$(cid "$cu")', '$(sid "$sv")', '$(sname "$sv")', $(mins "$sv"), $(price "$sv"), '$ts', '$ts'::timestamptz + make_interval(mins => $(mins "$sv")), '$st', now(), now());"
done <<EOF
$HISTORY
EOF
docker compose -f "$COMPOSE_FILE_PATH" exec -T postgres psql -q -U suite -d suite -v ON_ERROR_STOP=1 -c "$SQL"
echo "     inserted $(printf '%s\n' "$HISTORY" | wc -l | tr -d ' ') historical appointments"

echo "== dashboard: API vs manual computation"
FROM=$(day -14); TO=$(day 14)
# Every appointment of this demo: offset hour service status (API-created ones with their final status).
ALL="$HISTORY
2 14 CUT ANA scheduled
2 14 CUT BRUNO scheduled
3 14 MASSAGE DAVI scheduled
3 14 COLOR CARLA cancelled
0 0 EXPRESS ANA completed"
price_of() { if [ "$1" = EXPRESS ]; then echo 3000; else price "$1"; fi; }
EXPECT=$(printf '%s\n' "$ALL" | while read -r off hour sv cu st; do [ -n "$off" ] && echo "$sv $st $(price_of "$sv")"; done | awk '
  { n++; c[$2]++; if ($2=="completed") { rev+=$3; done_n++ } }
  END { printf "%d %d %d %d %d %d %d %d\n", n, c["scheduled"], c["completed"], c["cancelled"], c["no_show"], rev, (done_n? int(rev/done_n+0.5):0), done_n }')
set -- $EXPECT
E_TOTAL=$1; E_SCHED=$2; E_DONE=$3; E_CANC=$4; E_NS=$5; E_REV=$6; E_TICKET=$7
E_CANC_RATE=$(awk -v n="$E_CANC" -v t="$E_TOTAL" 'BEGIN { printf "%.4f", n/t }'); E_NS_RATE=$(awk -v n="$E_NS" -v t="$E_TOTAL" 'BEGIN { printf "%.4f", n/t }')
echo "   manual: total=$E_TOTAL scheduled=$E_SCHED completed=$E_DONE cancelled=$E_CANC no_show=$E_NS revenue=$E_REV ticket=$E_TICKET"
echo "   manual: revenue = $(printf '%s\n' "$ALL" | while read -r off hour sv cu st; do [ "$st" = completed ] && price_of "$sv"; done | paste -sd+ -) = $E_REV"
expect 200 "GET /v1/dashboard/summary?from=$FROM&to=$TO" -H "$(auth "$STAFF")" "$API/v1/dashboard/summary?from=$FROM&to=$TO"; body
check "appointments_total" "$E_TOTAL" "$(jq .appointments_total "$BODYF")"
check "by_status.scheduled" "$E_SCHED" "$(jq .by_status.scheduled "$BODYF")"
check "by_status.completed" "$E_DONE" "$(jq .by_status.completed "$BODYF")"
check "by_status.cancelled" "$E_CANC" "$(jq .by_status.cancelled "$BODYF")"
check "by_status.no_show" "$E_NS" "$(jq .by_status.no_show "$BODYF")"
check "revenue_cents (completed only)" "$E_REV" "$(jq .revenue_cents "$BODYF")"
check "average_ticket_cents" "$E_TICKET" "$(jq .average_ticket_cents "$BODYF")"
check "cancellation_rate ($E_CANC/$E_TOTAL)" "$E_CANC_RATE" "$(jq -r .cancellation_rate "$BODYF" | awk '{ printf "%.4f", $1 }')"
check "no_show_rate ($E_NS/$E_TOTAL)" "$E_NS_RATE" "$(jq -r .no_show_rate "$BODYF" | awk '{ printf "%.4f", $1 }')"
check "new_customers (4 created today)" 4 "$(jq .new_customers "$BODYF")"

expect 200 "GET /v1/dashboard/daily for yesterday" -H "$(auth "$STAFF")" "$API/v1/dashboard/daily?from=$(day -1)&to=$(day -1)"
check "yesterday appointments (Haircut+Hair color)" 2 "$(jq '.data[0].appointments' "$BODYF")"
check "yesterday revenue_cents (5000+20000)" 25000 "$(jq '.data[0].revenue_cents' "$BODYF")"
expect 200 "GET /v1/dashboard/daily over 29 days (zero-filled)" -H "$(auth "$STAFF")" "$API/v1/dashboard/daily?from=$FROM&to=$TO"
check "points in the series (no holes)" 29 "$(jq '.data | length' "$BODYF")"
check "sum of the daily revenue" "$E_REV" "$(jq '[.data[].revenue_cents] | add' "$BODYF")"

expect 200 "GET /v1/dashboard/top-services" -H "$(auth "$STAFF")" "$API/v1/dashboard/top-services?from=$FROM&to=$TO"
jq -r '.data[] | "     \(.name): \(.appointments) appointments, \(.completed) completed, \(.revenue_cents) cents"' "$BODYF"
check "top service (by realized revenue)" "Hair color" "$(jq -r '.data[0].name' "$BODYF")"
HC=$(printf '%s\n' "$ALL" | awk '$3=="COLOR" && $5=="completed" {n++} END {print n*20000}')
check "Hair color revenue_cents" "$HC" "$(jq '.data[0].revenue_cents' "$BODYF")"
expect 200 "GET /v1/dashboard/upcoming" -H "$(auth "$STAFF")" "$API/v1/dashboard/upcoming?limit=3"
printf '     next: %s\n' "$(jq -r '[.data[] | "\(.starts_at) \(.service_name)"] | join(" | ")' "$BODYF")"
expect 422 "invalid period (422)" -H "$(auth "$STAFF")" "$API/v1/dashboard/summary?from=2026-03-10&to=2026-03-01"; body

echo "== deletes (admin only) and integrity"
expect 403 "staff cannot delete a service (403)" -H "$(auth "$STAFF")" -X DELETE "$API/v1/services/$CUT"
expect 409 "admin: service with appointments (409 service_in_use)" -H "$(auth "$ADMIN")" -X DELETE "$API/v1/services/$CUT"; body
expect 204 "admin deletes the unused 'Retired service'" -H "$(auth "$ADMIN")" -X DELETE "$API/v1/services/$OLD"

echo "== rate limit on /v1/auth/* (10/min per IP)"
n=0; while [ $n -lt 12 ]; do
  code=$(curl -s -o "$BODYF" -w '%{http_code}' -H "$J" "$API/v1/auth/login" -d '{"email":"x@example.com","password":"nope-nope-nope"}'); n=$((n+1))
  [ "$code" = 429 ] && break
done
printf '%-4s after %s more attempts (Retry-After: %ss)\n' "$code" "$n" "$(curl -s -D - -o /dev/null -H "$J" "$API/v1/auth/login" -d '{}' | tr -d '\r' | awk -F': ' 'tolower($1)=="retry-after"{print $2}')"

if [ "$FAILED" = 0 ]; then echo "== demo finished: all dashboard numbers match the manual computation"; else echo "== demo finished WITH FAILURES"; exit 1; fi
