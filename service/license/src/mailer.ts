// Sends via Resend's REST API instead of Cloudflare's native Email Sending
// (which requires the Workers Paid plan). This only needs outbound fetch,
// which works on the Free plan — nothing here is Cloudflare-specific.
export interface MailerEnv {
  RESEND_API_KEY: string;
  FROM_EMAIL: string;
  FROM_NAME: string;
}

export async function sendMail(
  env: MailerEnv,
  opts: { to: string; subject: string; text: string; html: string }
): Promise<void> {
  const resp = await fetch("https://api.resend.com/emails", {
    method: "POST",
    headers: {
      Authorization: `Bearer ${env.RESEND_API_KEY}`,
      "content-type": "application/json",
    },
    body: JSON.stringify({
      from: `${env.FROM_NAME} <${env.FROM_EMAIL}>`,
      to: [opts.to],
      subject: opts.subject,
      text: opts.text,
      html: opts.html,
    }),
  });

  if (!resp.ok) {
    const body = await resp.text().catch(() => "");
    throw new Error(`Resend send failed (${resp.status}): ${body}`);
  }
}
