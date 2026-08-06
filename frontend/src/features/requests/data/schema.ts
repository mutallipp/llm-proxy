import { z } from 'zod';
import { pageInfoSchema } from '@/gql/pagination';
import { apiKeySchema } from '@/features/apikeys/data/schema';
import { channelSchema } from '@/features/channels/data';
import { usageLogSchema } from './usage-logs-schema';

// Request Status
export const requestStatusSchema = z.enum(['pending', 'processing', 'completed', 'failed', 'canceled']);
export type RequestStatus = z.infer<typeof requestStatusSchema>;

// Request Source
export const requestSourceSchema = z.enum(['api', 'playground', 'test']);
export type RequestSource = z.infer<typeof requestSourceSchema>;

// Request Execution Status
export const requestExecutionStatusSchema = z.enum(['pending', 'processing', 'completed', 'failed', 'canceled']);
export type RequestExecutionStatus = z.infer<typeof requestExecutionStatusSchema>;

const bodyAvailabilityValues = ['unknown', 'available', 'unavailable'] as const;
const chunksAvailabilityValues = ['unknown', 'available', 'unavailable', 'not_applicable'] as const;
export const requestBodyAvailabilitySchema = z.enum(bodyAvailabilityValues).default('unknown');
export const requestResponseBodyAvailabilitySchema = z.enum(bodyAvailabilityValues).default('unknown');
export const requestResponseChunksAvailabilitySchema = z.enum(chunksAvailabilityValues).default('unknown');
export const requestResponseChunksPersistedAvailabilitySchema = z.enum(chunksAvailabilityValues).default('unknown');
export type BodyAvailability = z.infer<typeof requestBodyAvailabilitySchema>;
export type ChunksAvailability = z.infer<typeof requestResponseChunksAvailabilitySchema>;

// Request Execution
export const requestExecutionSchema = z.object({
  id: z.string(),
  createdAt: z.coerce.date(),
  updatedAt: z.coerce.date(),
  // userID: z.number(),
  // requestID: z.string(),
  // channelID: z.number(),
  channel: channelSchema.partial().nullable().optional(),
  modelID: z.string(),
  requestHeaders: z.any().nullable().optional(),
  requestBody: z.any(), // JSONRawMessage
  responseBody: z.any().nullable(), // JSONRawMessage
  responseChunks: z.array(z.any()).nullable(), // [JSONRawMessage!]
  responseChunksLive: z.boolean().default(false),
  requestBodyAvailability: requestBodyAvailabilitySchema,
  responseBodyAvailability: requestResponseBodyAvailabilitySchema,
  responseChunksAvailability: requestResponseChunksAvailabilitySchema,
  responseChunksPersistedAvailability: requestResponseChunksPersistedAvailabilitySchema,
  errorMessage: z.string().nullable(),
  responseStatusCode: z.number().nullable().optional(),
  status: requestExecutionStatusSchema,
  format: z.string().optional(),
  metricsLatencyMs: z.number().nullable().optional(),
  metricsFirstTokenLatencyMs: z.number().nullable().optional(),
  metricsReasoningDurationMs: z.number().nullable().optional(),
  requestURL: z.string().nullable().optional(),
  passThroughApplied: z.boolean().optional(),
});
export type RequestExecution = z.infer<typeof requestExecutionSchema>;

// Request
export const requestSchema = z.object({
  id: z.string(),
  createdAt: z.coerce.date(),
  updatedAt: z.coerce.date(),
  // apiKeyID: z.string().optional().nullable(),
  apiKey: apiKeySchema.partial().nullable().optional(),
  // channelID: z.string().optional().nullable(),
  channel: channelSchema.partial().nullable().optional(),
  source: requestSourceSchema,
  testOriginType: z.enum(['channel', 'model', 'adapter']).nullable().optional(),
  testOriginID: z.number().nullable().optional(),
  testOriginLabel: z.string().nullable().optional(),
  modelID: z.string(),
  reasoningEffort: z.string().nullable().optional(),
  contentSaved: z.boolean().optional(),
  contentStorageKey: z.string().nullable().optional(),
  requestHeaders: z.any().nullable().optional(),
  requestBody: z.any().nullable().optional(), // JSONRawMessage
  responseBody: z.any().nullable().optional(), // JSONRawMessage
  responseChunks: z.array(z.any()).nullable().optional(), // [JSONRawMessage!]
  responseChunksLive: z.boolean().default(false),
  requestBodyAvailability: requestBodyAvailabilitySchema,
  responseBodyAvailability: requestResponseBodyAvailabilitySchema,
  responseChunksAvailability: requestResponseChunksAvailabilitySchema,
  responseChunksPersistedAvailability: requestResponseChunksPersistedAvailabilitySchema,
  status: requestStatusSchema,
  format: z.string().optional(),
  clientIP: z.string().nullable().optional(),
  stream: z.boolean().nullable(),
  metricsLatencyMs: z.number().nullable().optional(),
  metricsFirstTokenLatencyMs: z.number().nullable().optional(),
  metricsReasoningDurationMs: z.number().nullable().optional(),
  executions: z
    .object({
      edges: z.array(
        z.object({
          node: requestExecutionSchema.partial().nullable().optional(),
          cursor: z.string(),
        })
      ),
      pageInfo: pageInfoSchema,
      totalCount: z.number(),
    })
    .optional(),
  usageLogs: z
    .object({
      edges: z
        .array(
          z.object({
            node: usageLogSchema.partial().nullable().optional(),
            cursor: z.string().optional(),
          })
        )
        .optional(),
      pageInfo: pageInfoSchema.optional(),
    })
    .optional()
    .nullable(),
});

export type Request = z.infer<typeof requestSchema>;

// Request Connection (for pagination)
export const requestConnectionSchema = z.object({
  edges: z.array(
    z.object({
      node: requestSchema,
      cursor: z.string(),
    })
  ),
  pageInfo: pageInfoSchema,
  totalCount: z.number(),
});
export type RequestConnection = z.infer<typeof requestConnectionSchema>;

// Request Execution Connection (for pagination)
export const requestExecutionConnectionSchema = z.object({
  edges: z.array(
    z.object({
      node: requestExecutionSchema,
      cursor: z.string(),
    })
  ),
  pageInfo: pageInfoSchema,
  totalCount: z.number(),
});
export type RequestExecutionConnection = z.infer<typeof requestExecutionConnectionSchema>;
