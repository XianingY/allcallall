export interface Pagination {
  total: number;
  limit: number;
  offset: number;
  has_more: boolean;
}

export interface PageRequest {
  limit?: number;
  offset?: number;
}

export const PAGE_SIZE = 50;
