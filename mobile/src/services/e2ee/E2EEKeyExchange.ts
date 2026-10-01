// Re-export: the key-exchange handshake is shared with the web client, so both
// ends negotiate with the same implementation. See @allcallall/shared.
export {
  E2EEKeyExchange,
  type DataChannelLike,
  type E2EEKeyExchangeCallbacks,
  type KeyExchangeMessage,
  type KeyExchangeRole,
} from "@allcallall/shared";
