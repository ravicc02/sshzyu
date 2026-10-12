/** Refresh admin attention indicators after a successful draw mutation. */
export const LOTTERY_DRAWS_UPDATED = 'lottery-draws-updated'
export function notifyLotteryDrawsUpdated() {
  window.dispatchEvent(new Event(LOTTERY_DRAWS_UPDATED))
}
