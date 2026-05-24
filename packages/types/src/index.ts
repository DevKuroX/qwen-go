export type AccountStatus = 'VALID' | 'RATE_LIMITED' | 'SOFT_ERROR' | 'CIRCUIT_OPEN' | 'HALF_OPEN' | 'BANNED' | 'PENDING_REFRESH';
export interface Account {
  email: string;
  password: string;
  token: string;
  username: string;
  status: AccountStatus;
  inflight: number;
}
