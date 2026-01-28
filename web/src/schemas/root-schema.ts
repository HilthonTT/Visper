import z from "zod";

export const rootSearchSchema = z.object({
  secureCode: z.string().optional(),
  joinCode: z.string().optional(),
});

export type RootSearchValues = z.infer<typeof rootSearchSchema>;
