usage() {
  echo "${0} <initial-size> <step-size> <target-size> [<sleep-duration-seconds>]"
}

if [[ "$#" -ne 3 ]] && [[ "$#" -ne 4 ]]; then
  usage
  exit 1
fi

declare -ir initial_size="${1}"
declare -ir step_size="${2}"
declare -ir target_size="${3}"
declare -ir sleep_duration_sec="${4:-7}"

echo "Scaling from ${initial_size} to ${target_size} by ${step_size} every ${sleep_duration_sec} seconds"

for ((i=${initial_size}; i<=${target_size}; i+=${step_size})); do
  current_size="${i}"
  echo "Scaling to ${current_size} loadbots"
  echo "kubectl scale rc vegeta --replicas=${current_size}"
  kubectl scale rc vegeta --replicas="${current_size}"
  sleep "${sleep_duration_sec}"
done
