import { createClient, ConnectError, Code } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { LedgerService } from './gen/beancount/v1/ledger_pb';

export const api = createClient(
  LedgerService,
  createConnectTransport({
    baseUrl:
      typeof location === 'undefined' ? 'http://localhost' : location.origin,
    fetch: (input, init) =>
      fetch(input, { ...init, credentials: 'same-origin' })
  })
);
export const errorMessage = (error: unknown) =>
  error instanceof ConnectError
    ? error.rawMessage
    : error instanceof Error
      ? error.message
      : 'Something went wrong. Please retry.';
export const isSignedOut = (error: unknown) =>
  error instanceof ConnectError && error.code === Code.Unauthenticated;
export const isConflict = (error: unknown) =>
  error instanceof ConnectError && error.code === Code.Aborted;
