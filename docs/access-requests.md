# Approval-based Emby access

Overmynd can handle server access requests directly. Wizarr is not needed. This
feature is off by default and does not change public media observation or Seerr
request permissions.

## Set up

1. In Emby, create a dedicated **disabled, non-administrator template user**.
   Set its library, playback, remote-access, and other permissions to those new
   members should inherit. Keep it disabled. Copy its user ID from the user page
   URL. This template is required: Overmynd copies its policy during creation so
   the new user starts disabled, before setting a random temporary password.
2. Create an Emby API key. The Emby server must support `CopyFromUserId` with
   `UserCopyOptions: ["UserPolicy"]` on `POST /Users/New` and Connect linking.
3. Publish Overmynd through HTTPS. In **Settings > Server access requests > Access
   and email settings**, enter that public URL, the internal Emby URL, API key,
   and template user ID. Use the URL of Overmynd, not a Wizarr invite URL.
4. Enter your provider's SMTP hostname, port, encryption, mailbox login, and
   password. Both TLS and STARTTLS require a valid server certificate. An alias
   can be the sender address if the mailbox is authorized to send as that alias;
   the alias is not necessarily an SMTP login. Enter your own notification email.
   For Spaceship, use the actual SMTP settings supplied with your email plan.
5. Save and use **Send test email**. Check receipt, then enable public access
   requests. The **Request access** button appears at the lower left.

Saved API and SMTP passwords are never returned to the browser. Leave their
fields empty to preserve them. They are stored in the configuration database, so
protect `/config` and its backups as you do the existing integration credentials.

## Applicant and approval flow

- Applicant creates/verifies an Emby Connect account on Emby's own site, then
  submits their name, notification email, desired local username, and Connect
  username. Overmynd never asks for their Connect password.
- Overmynd holds the request and emails the applicant and administrator. No Emby
  user is created before approval.
- Administrator signs into Overmynd and approves or declines in the approval
  queue. Approval copies the disabled template policy, secures the account with
  a random password, and requests the Connect link.
- Applicant gets an expiring setup link by email. They confirm any Connect email,
  then choose their local Emby password. The account is enabled only after setup
  succeeds and Connect no longer reports a pending confirmation.
- Applicant receives a completion notice. A decline sends a decline notice.

Setup links last 24 hours and work once. Only their hashes are saved. Resending a
link invalidates the previous one. The local password is sent to Emby during
setup and is never stored in Overmynd. Request data is visible only to admins.

Connect verification reads the existing user's `ConnectUserName` and
`ConnectLinkType` plus `GET /Connect/Pending`; it never recreates the link during
password setup. The linked identity must match the approved Connect identity
(case-insensitively). An unrecognized pending-link response keeps setup blocked
rather than assuming confirmation.

## Failures and retries

The queue displays setup and delivery errors. Email failure does not undo an
approval or cause another Emby account to be created. Use **Resend notices** or
**Send fresh setup link** after fixing SMTP. Delivery means acceptance by the
SMTP server; check inbox/spam or your provider's delivery logs for final delivery.
Notices are sent during the action, with bounded network timeouts; there is no
background automatic delivery retry. After an interrupted action, refresh the
queue and resend as needed.

Use **Correct contact details** for a pending/incomplete request with an incorrect
email or Connect username, then approve/retry. Successful approval locks these
details. Existing Emby usernames are never adopted or reset. An uncertain create
response therefore requires checking Emby for a partial account and resolving
the conflict before retrying. A recorded user ID is reused on retries.

If you delete a provisioned Emby user to test again, use **Reset after Emby
deletion** on the existing request. Overmynd checks both the deleted user's
endpoint and the Emby user list before clearing its ID and all old setup links.
It does not delete or modify any Emby users. Approve the request again and use the
new emails. Public setup links can never recreate a deleted user automatically.

Restarting Overmynd makes interrupted provisioning/setup retryable. Provisioned
accounts remain disabled until setup succeeds. Do not run multiple Overmynd
instances against the same configuration database. Changing the configured Emby
server is blocked once requests have provisioned users.

Requests are deduplicated by email and local username, including declined requests;
the public response does not reveal the matching record. Public submissions and
setup attempts are limited per connecting IP, and unfinished requests are capped
at 500. With a reverse proxy, clients can share that limit. The endpoints require
the same origin/custom header protections as other Overmynd writes.

## Verify on your server

Use a test applicant and your own email first. Confirm receipt, approval,
disabled-before-setup behavior in Emby, Connect confirmation, library permissions,
local password setup, and actual sign-in. Automated tests use a simulated Emby
server and email sender; your Emby version and SMTP provider still need this
end-to-end check before inviting others.

API references: [Create user](https://dev.emby.media/reference/RestAPI/UserService/postUsersNew.html),
[Connect link](https://dev.emby.media/reference/RestAPI/ConnectService/postUsersByIdConnectLink.html).
