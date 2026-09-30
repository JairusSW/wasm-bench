package protocol

// This memory window is deliberately broader than the sum of per-call timers:
// inter-case input writes and oracle checks must preserve the ordered sequence.
const VectorFirstCallWindowDenominator = "ordered_calls_with_intercase_input_and_verification_excluding_last_oracle_release_and_barriers"

const VectorFirstCallPhasesPolicy = "fresh initialized instance; pointers and first input prepared before before_first_call; first_call_returned follows last ordered call before its output oracle; memory window includes intercase input writes and oracle checks, unlike sequence_call_sum timers; final oracle and verified instance release follow returned barrier; compiled module retained; no forced reclamation"
