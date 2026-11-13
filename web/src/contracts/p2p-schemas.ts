import { z } from "zod";

const P2PPortMapStatusSchema = z.object({
  state: z.enum(["off", "probing", "mapped", "unavailable"]),
  protocol: z.enum(["nat-pmp", "pcp", "upnp"]).optional(),
  externalPort: z.number().int().min(1).max(65535).optional(),
});

const P2PPeerInfoSchema = z.object({
  id: z.string(),
  state: z.string(),
  candidateType: z.string().optional(),
  ipFamily: z.string().optional(),
  rttMs: z.number().int().min(0).optional(),
  channels: z.number().int().min(0),
  createdAt: z.string(),
});

export const P2PStatusSchema = z.object({
  supported: z.boolean(),
  udpPort: z.number().int().min(0).max(65535),
  udpBindings: z.array(
    z.object({
      network: z.enum(["udp4", "udp6"]),
      port: z.number().int().min(1).max(65535),
    }),
  ),
  capabilities: z.object({
    networks: z.array(z.enum(["udp4", "udp6"])),
    iceMode: z.enum(["full", "unsupported"]),
    turn: z.boolean(),
  }),
  enabled: z.boolean(),
  portMapping: z.boolean(),
  stunServers: z.array(z.string()),
  source: z.enum(["config", "web"]),
  storageAvailable: z.boolean(),
  storageError: z.string().optional(),
  portMap: P2PPortMapStatusSchema,
  activePeers: z.number().int().min(0),
  peers: z.array(P2PPeerInfoSchema),
});
export type P2PStatus = z.infer<typeof P2PStatusSchema>;

export const P2PSettingsUpdateSchema = z
  .object({
    enabled: z.boolean(),
    portMapping: z.boolean(),
    stunServers: z.array(z.string()).max(8),
  })
  .strict();
export type P2PSettingsUpdate = z.infer<typeof P2PSettingsUpdateSchema>;

const P2PICECandidateSchema = z.object({
  candidate: z.string(),
  sdpMid: z.string().nullable().optional(),
  sdpMLineIndex: z.number().int().min(0).nullable().optional(),
});
export type P2PICECandidate = z.infer<typeof P2PICECandidateSchema>;

export const P2PSignalResponseSchema = z.object({
  peerId: z.string(),
  sdp: z.string().optional(),
  candidates: z.array(P2PICECandidateSchema),
  stunServers: z.array(z.string()),
});

export const P2PPingResponseSchema = z.object({
  seq: z.number().int(),
  serverTs: z.number().int(),
});

export const P2PControlMessageSchema = z.discriminatedUnion("type", [
  z.object({
    type: z.literal("hello"),
    proto: z.number().int(),
    appVersion: z.string(),
    features: z.array(z.string()).optional(),
    peerId: z.string().optional(),
    candidateType: z.string().optional(),
    ipFamily: z.string().optional(),
    portMapped: z.boolean().optional(),
  }),
  z.object({
    type: z.literal("ping"),
    seq: z.number().int(),
    clientTs: z.number(),
  }),
  z.object({
    type: z.literal("pong"),
    seq: z.number().int(),
    clientTs: z.number(),
  }),
  z.object({
    type: z.literal("peer_meta"),
    candidateType: z.string().optional(),
    ipFamily: z.string().optional(),
    rttMs: z.number().int().min(0).optional(),
  }),
]);

export const P2PRPCRequestHeaderSchema = z.object({
  method: z.string(),
  path: z.string(),
  headers: z.record(z.string(), z.string()),
  bodyLength: z.number().int().min(0),
});

export const P2PRPCResponseHeaderSchema = z.object({
  status: z.number().int(),
  statusText: z.string(),
  headers: z.record(z.string(), z.string()),
});
