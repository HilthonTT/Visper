# Security model

What Visper's end-to-end encryption protects, against whom, and where it
stops. The implementation is in [`sdk/go/roomcrypto.go`](../sdk/go/roomcrypto.go)
and [`sdk/go/envelope.go`](../sdk/go/envelope.go), mirrored for the browser in
[`clients/web/src/lib/room-keys.ts`](../clients/web/src/lib/room-keys.ts). Both
are tested against the same vectors,
[`sdk/go/testdata/crypto-vectors.json`](../sdk/go/testdata/crypto-vectors.json).

## Keys

| Key | Algorithm | Made by | Who has it |
| --- | --- | --- | --- |
| Room key | 256-bit, NaCl secretbox (XSalsa20-Poly1305) | the client creating the room, and the owner's client on every rekey | members, through invites and rekeys |
| Signing key | Ed25519 | each CLI install; each web client page load (or remembered room) | that client only |
| Device key | X25519 | each CLI install; each web client page load (or remembered room) | that client only |

None of these ever reaches the server in usable form. The CLI keeps its keys
on disk and the web client can be asked to; see
[Stored keys](#stored-keys). Otherwise keys live in memory and disappear when
the browser tab reloads.

- **Invites** carry the room key in the invite code (`<join code>.<key>`) or
  in the invite link's `#key=` fragment, which browsers never send to a
  server.
- **Key cards** tell the room which signing and device keys belong to a
  member. A card is signed by its signing key and sealed with the room key,
  so the server can't forge one. The API files it under the uploading user's
  ID, so a member can't publish one for someone else.
- **Rekeys**: when a member leaves or is kicked, the owner's client makes a
  new room key and seals it to each remaining member's device key: X25519,
  then HKDF-SHA256, then AES-256-GCM. It signs the whole set. Members check
  the owner's signature, switch keys and publish a new card. Older keys are
  kept so history stays readable.

## Envelopes

Everything a member encrypts (message text, edits, reactions and key cards)
is a signed envelope, sealed with the room key:

```
secretbox( 0x02 || Ed25519 signature || {"v":2,"k","f","m","n","ts","t","h"} )
```

| Field | Meaning | Stops |
| --- | --- | --- |
| `k` | kind: message, reaction or card | a reaction shown as a message |
| `f` | sender's user ID, checked against the signature | anyone writing as someone else |
| `m` | the message an edit or reaction belongs to | moving an edit or reaction to another message |
| `n` | random nonce of a new message | replaying a message under a new ID |
| `ts` | when it was written, which must be within 15 minutes of the server's time for it | replaying old content later, rolling an edit back |
| `h` | SHA-256 of the encrypted file an image message points to | swapping the file behind an image |

Unsigned (version 1) content is refused, so nobody can downgrade to it.

## What the server can and can't do

The server, or anyone who compromises it, **cannot**:

- read messages, reactions, edits or shared files;
- read or forge key cards, so it can't learn a member's device key or
  plant a fake one;
- make content look as if another member wrote it;
- edit someone's message, or show an edit for a different message;
- replay a message under a new ID, or replay it more than 15 minutes later;
- swap the file an image message points to;
- give members a new room key of its own choosing, because rekeys must
  carry the owner's signature.

The server **can still**:

- **See metadata**: who is in which room, when they post, message sizes,
  reactions per message, IP addresses, and the usernames people pick.
- **Withhold, delete or delay** messages, cards and rekeys. Clients can't
  tell a message that was never sent from one the server dropped.
- **Roll back reactions** to an earlier state, and replay a message within
  the 15-minute window to a client that never saw the original.
- **Lie about who owns a room** when a client first enters it. Clients pin
  the owner they see on entry and accept rekeys only from that owner, but the
  first answer has to be trusted.
- **Serve modified web client code.** A browser runs whatever JavaScript the
  site sends, so whoever controls that site can make the page leak keys. The
  CLI doesn't have this problem. Use it, or host the web client yourself,
  if the server isn't trusted.

## Members

A member holds the room key, so they can read everything sent while they
were in the room. Beyond that:

- **They can't write as someone else.** Signatures are checked against the
  sender's key card.
- **Once they leave or are kicked, they can't read what comes next.** That
  holds after the owner's client rotates the key, which it does as soon as it
  sees the departure, or when the owner next opens the room. Until then, a
  departed member who can still get ciphertext (for example, from a leaked
  server database) could read it. If the owner never comes back (a web
  owner who reloaded without remembering the room can't), the key is never
  rotated.
- **A departed member working with the server** can get the new key, unless
  the owner has verified the remaining members. The server can show the owner
  a key card the departed member made for a remaining member, and the owner
  then seals the new key to it. The same forged card would also let them sign
  messages in that member's name. See [Verifying keys](#verifying-keys).
- **Members without a key card** (a client that couldn't publish one) don't
  get rotated keys. The owner's client names them, and they need a new
  invite.

## Verifying keys

Each key card has a fingerprint: six groups of five digits, derived from the
member's user ID and their signing and device keys:

```
SHA-512("visper/keycard/v1\n<user ID>\n<signing key>\n<device key>\n")
```

The keys are base64url. Group *i* is bytes 5*i* to 5*i*+4, read as a
big-endian number, mod 100000. To verify someone, open their key
(CLI: `ctrl+s`, pick them, `ctrl+k`; web: click their name under
**members**). Then ask them to read out *their own* key over a call or in
person, and mark them verified only if the digits match. Don't compare
fingerprints through the room itself, since the server relays it.

Verifying a member pins that card in your client:

- Only the pinned card is accepted for their signatures. Messages signed with
  another card show as unverified. If you verify the owner, rekeys signed with
  another card are refused.
- As owner, your rekeys seal the new key to the pinned card's device, never
  to a newer card in their name.
- If a different card appears for them, they're marked as having a changed
  key (`!`) and you're warned. Their client may have restarted, or someone may
  be posing as them. Compare again before you trust the new key.

Verification lasts as long as your identity. The CLI keeps its identity and
the members you verified between runs (see [Stored keys](#stored-keys)). The
web client keeps them only for rooms you ask it to remember; otherwise its
fingerprint changes each time the page loads and verification is lost. When
someone's fingerprint changes, verify them again.

## Stored keys

**CLI.** The CLI keeps one identity on disk so you can quit, come back as the
same member and reopen rooms from the Join page without a new invite. It is
`visper/identity.json` in your user config directory (`~/.config` on Linux,
`~/Library/Application Support` on macOS, `%AppData%` on Windows), written
owner-only (mode 0600; on Windows, it relies on the per-user profile folder).
It holds, **unencrypted**:

- your user token (your credential with the API);
- your signing and device private keys;
- for each room you are in: its keys, the owner pinned when you first
  joined, and the members you verified.

Anyone who can read this file can read those rooms and post as you. Two CLIs
on the same machine share it and act as the same member. `visper -forget`
deletes it. Quitting (`ctrl+c`) keeps you in your rooms; `ctrl+b` leaves (or,
as owner, deletes the room). Rooms are dropped from the file when you leave,
are kicked, the room is deleted or expires. If the file can't be written,
the CLI falls back to an in-memory identity and leaves its rooms on quit, as
before.

Because the owner's CLI now returns as the same identity, a key rotation
that was missed while the owner was away happens the next time they open the
room.

**Web client.** Nothing is stored unless you tick **remember the room on
this browser** (or press **remember** in the room bar). Then the room's keys,
your identity for that room and your verified members go into the browser's
IndexedDB, **unencrypted**. Anyone with access to that browser profile can read
them, and so can any script the site serves: a modified web client can now
take stored keys, not only those of an open tab. **forget** deletes them;
leaving, being kicked, deletion and expiry do too. Reopening a remembered room
checks that the API still knows the browser as the same member and that the
owner is the one pinned on first entry.

## Invites

An invite code or link contains the room key. Anyone who has it can read the
room until the next rotation. Share invites over a channel you trust. Old
invites stop working after a rotation: newcomers can join, but can't read
anything.

## AI rewrite

**The CLI's AI rewrite (ctrl+a) is not end-to-end encrypted.** It sends your
draft, unencrypted, to the Visper API's AI service, which passes it to an
Ollama model and briefly stores it while it processes it. The CLI asks before
the first rewrite of each session. Don't use it for anything you wouldn't
send to the server operator.

## Reporting

See [SECURITY.md](../.github/SECURITY.md).
