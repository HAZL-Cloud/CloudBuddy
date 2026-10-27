import type {
  Action,
  AgentName,
  Approval,
  ChatItem,
  Connection,
  ConnectionsResponse,
  ConversationDetail,
  Project,
  RunResult,
  Settings,
  Step,
  StepStatus,
  Tag,
  Via,
} from '../types'

// The four conversations from docs/mockup.html, as the API would return them. None of these accounts are real.
// Times are clock times on a given day, so the cards read like the mockup whatever day it is.

export function at(hms: string, daysAgo = 0): string {
  const [h = 0, m = 0, s = 0] = hms.split(':').map(Number)
  const d = new Date()
  d.setDate(d.getDate() - daysAgo)
  d.setHours(h, m, s, 0)
  return d.toISOString()
}

interface StepSpec {
  id: string
  kind?: 'cloud' | 'local'
  provider: string
  product: string
  action: Action
  title: string
  resource: string
  facts?: [string, string][]
  tags?: Tag[]
  via?: Via
  generic?: boolean
  guessWord?: string
  status?: StepStatus
  time: string
  day?: number
  ms?: number
  api: string
  command: string
  why: string
  output?: string
  maskedNote?: string
  uses?: { fromStepId: string; what: string }[]
  approval?: Approval
}

function mk(s: StepSpec): Step {
  const kind = s.kind ?? 'cloud'
  const status = s.status ?? 'done'
  const startedAt = at(s.time, s.day)
  const finished = status === 'done' || status === 'failed'
  return {
    id: s.id,
    seq: 0,
    kind,
    provider: s.provider,
    product: s.product,
    action: s.action,
    tags: s.tags ?? [],
    title: s.title,
    titleSource: s.generic ? 'agent' : 'adapter',
    resource: s.resource,
    facts: s.facts ?? [],
    via: s.via ?? 'cli',
    recognizedBy: kind === 'local' ? ['local'] : s.generic ? ['generic'] : ['builtin'],
    generic: s.generic ?? false,
    guessWord: s.guessWord,
    command: s.command,
    api: s.api,
    why: s.why,
    status,
    startedAt,
    endedAt: finished && s.ms !== undefined ? new Date(Date.parse(startedAt) + s.ms).toISOString() : undefined,
    durationMs: finished ? s.ms : undefined,
    output: s.output ?? '',
    outputTruncated: false,
    maskedNote: s.maskedNote,
    uses: s.uses ?? [],
    approval: s.approval,
  }
}

function ask(
  id: string,
  state: Approval['state'],
  reason: string,
  o: { loss?: string[]; lossTitle?: string; asked: string; decided?: string; day?: number },
): Approval {
  return {
    id,
    state,
    reason,
    loss: o.loss,
    lossTitle: o.lossTitle,
    requestedAt: at(o.asked, o.day),
    decidedAt: o.decided ? at(o.decided, o.day) : undefined,
  }
}

type ChatSpec = ['user' | 'agent', string] | ['steps', string[]] | ['approval', string]

function conv(
  head: {
    id: string
    title: string
    agent: AgentName
    model: string
    workdir: string
    projectId?: string
    extraFolders?: string[]
    subtitle: string
    status: ConversationDetail['status']
    day?: number
    result?: RunResult
  },
  steps: Step[],
  chatSpec: ChatSpec[],
): ConversationDetail {
  steps.forEach((s, i) => {
    s.seq = i + 1
  })
  const first = steps[0]?.startedAt ?? new Date().toISOString()
  const chat: ChatItem[] = chatSpec.map((c, i) => {
    const base = { id: `${head.id}-c${i + 1}`, at: first }
    if (c[0] === 'steps') return { ...base, type: 'steps', stepIds: c[1] }
    if (c[0] === 'approval') return { ...base, type: 'approval', stepId: c[1] }
    return { ...base, type: c[0], text: c[1] }
  })
  const last = steps[steps.length - 1]
  return {
    id: head.id,
    title: head.title,
    agent: head.agent,
    model: head.model,
    workdir: head.workdir,
    projectId: head.projectId,
    extraFolders: head.extraFolders ?? [],
    status: head.status,
    updatedAt: last?.endedAt ?? last?.startedAt ?? first,
    stepCount: steps.length,
    waitingCount: steps.filter((s) => s.status === 'waiting').length,
    subtitle: head.subtitle,
    chat,
    steps,
    result: head.result,
  }
}

export function newsletterConv(): ConversationDetail {
  const steps = [
    mk({
      id: 'n1', provider: 'Resend', product: 'Domains', action: 'read', via: 'mcp', time: '20:31:05', ms: 400,
      title: 'Checked sending domains in Resend', resource: 'Domains · 1 found',
      facts: [['shipyard.dev', 'Not added yet']],
      api: 'resend · list_domains', command: 'list_domains({})',
      why: 'Emails can only go out from a domain Resend has verified.',
      output: '{\n  "data": [\n    { "name": "mail.shipyard-old.com", "status": "verified" }\n  ]\n}',
    }),
    mk({
      id: 'n2', provider: 'Resend', product: 'Domains', action: 'create', via: 'mcp', time: '20:31:09', ms: 900,
      title: 'Added shipyard.dev to Resend', resource: 'Domains · shipyard.dev',
      facts: [['Region', 'us-east-1'], ['DNS records', '3 to add']],
      api: 'resend · create_domain', command: 'create_domain({ "name": "shipyard.dev" })',
      why: 'Resend hands back the DNS records that prove you own the domain.',
      output: '{\n  "id": "d91cfa3e-5b0c-4f7a",\n  "name": "shipyard.dev",\n  "status": "not_started",\n  "records": ["MX", "TXT (SPF)", "TXT (DKIM)"]\n}',
    }),
    mk({
      id: 'l1', kind: 'local', provider: 'Local', product: 'Files', action: 'create', via: 'file', time: '20:31:30', ms: 40,
      title: 'Saved the 3 DNS records to a file', resource: 'resend-dkim.json',
      api: 'file · write', command: 'write resend-dkim.json',
      why: 'The Cloudflare call reads the records from this file.',
    }),
    mk({
      id: 'n3', provider: 'Cloudflare', product: 'DNS', action: 'create', via: 'api', time: '20:31:40', ms: 2700,
      title: 'Added 3 email DNS records', resource: 'DNS · zone shipyard.dev',
      facts: [['Records', 'MX, SPF, DKIM'], ['Proxy', 'Off (DNS only)']],
      uses: [{ fromStepId: 'n2', what: '3 DNS records' }],
      api: 'POST /zones/:id/dns_records × 3',
      command: 'curl -X POST "https://api.cloudflare.com/client/v4/zones/$ZONE_ID/dns_records" \\\n  -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \\\n  --data @resend-dkim.json\n# … and 2 more records',
      why: 'Resend checks these records to confirm shipyard.dev is yours.',
      output: '{ "success": true, "result": { "type": "TXT", "name": "resend._domainkey.shipyard.dev" } }\n… 2 more records added',
    }),
    mk({
      id: 'n4', provider: 'Resend', product: 'Domains', action: 'read', via: 'mcp', time: '20:33:02', ms: 300,
      title: 'Checked shipyard.dev is verified', resource: 'Domains · shipyard.dev',
      facts: [['Status', 'Verified'], ['Waited', '1 min 20 s']],
      api: 'resend · get_domain', command: 'get_domain({ "id": "d91cfa3e-5b0c-4f7a" })',
      why: 'Sending fails until the domain is verified.',
      output: '{\n  "name": "shipyard.dev",\n  "status": "verified"\n}',
    }),
    mk({
      id: 'n5', provider: 'Stripe', product: 'Prices', action: 'create', tags: ['live', 'irreversible'], time: '20:38:44', ms: 800,
      title: 'Created the $8/month plan', resource: 'Prices · Shipyard Weekly',
      facts: [['Price', '$8.00 / month'], ['Price ID', 'price_1QxR8sKj2LmN7pQ']],
      api: 'stripe · prices create',
      command: 'stripe prices create --live \\\n  --currency usd --unit-amount 800 \\\n  -d "recurring[interval]=month" \\\n  -d "product_data[name]=Shipyard Weekly"',
      why: 'The plan your readers subscribe to.',
      output: '{\n  "id": "price_1QxR8sKj2LmN7pQ",\n  "unit_amount": 800,\n  "recurring": { "interval": "month" },\n  "livemode": true\n}',
      approval: ask('ap-n5', 'approved', "It runs in live mode, and a live price's amount can't be changed after it's created.", {
        lossTitle: 'What will happen',
        loss: ['Real customers can buy it right away', "The $8 amount can't be edited later, only archived"],
        asked: '20:38:20', decided: '20:38:44',
      }),
    }),
    // D-025: live mode always asks. The mockup's step 6 ran without asking, which was wrong.
    mk({
      id: 'n6', provider: 'Stripe', product: 'Webhooks', action: 'create', tags: ['live'], time: '20:38:51', ms: 600,
      title: 'Created a webhook for new subscribers', resource: 'Webhooks · newsletter-api',
      facts: [['Sends', 'customer.subscription.created'], ['To', 'newsletter-api on Cloud Run']],
      api: 'stripe · webhook_endpoints create',
      command: 'stripe webhook_endpoints create --live \\\n  --url https://newsletter-api-x7k2.a.run.app/stripe \\\n  -d "enabled_events[]=customer.subscription.created"',
      why: 'Tells the signup API when someone subscribes.',
      output: '{\n  "id": "we_1QxR9aKj2T4uV",\n  "secret": "whsec_••••••••••••••••••••7Qa",\n  "livemode": true\n}',
      maskedNote: 'CloudBuddy hid the webhook secret before saving this output.',
      approval: ask('ap-n6', 'approved', 'It runs in live mode, and live mode always asks.', {
        lossTitle: 'What will happen',
        loss: ['Stripe starts sending real subscriber events to newsletter-api right away'],
        asked: '20:38:47', decided: '20:38:51',
      }),
    }),
    mk({
      id: 'l2', kind: 'local', provider: 'Local', product: 'Files', action: 'read', via: 'file', time: '20:39:50', ms: 30,
      title: "Looked up the signup API's name", resource: 'deploy/service.yaml',
      api: 'file · read', command: 'read deploy/service.yaml',
      why: 'The deploy needs the exact service name.',
      output: 'name: newsletter-api\nregion: us-central1',
    }),
    mk({
      id: 'n7', provider: 'Google Cloud', product: 'Cloud Run', action: 'change', generic: true, guessWord: 'update', time: '20:40:12', ms: 21400,
      title: 'Gave the signup API the plan and secret', resource: 'Cloud Run · newsletter-api',
      facts: [['Region', 'us-central1'], ['New version', 'newsletter-api-00012']],
      uses: [{ fromStepId: 'n5', what: 'plan ID' }, { fromStepId: 'n6', what: 'webhook secret' }],
      api: 'gcloud run services update',
      command: 'gcloud run services update newsletter-api \\\n  --region us-central1 \\\n  --update-env-vars STRIPE_PRICE_ID=price_1QxR8sKj2LmN7pQ,STRIPE_WEBHOOK_SECRET=whsec_••••7Qa',
      why: "The signup API needs the plan ID to start checkout, and the secret to trust Stripe's messages.",
      output: 'Deploying... Done.\nService [newsletter-api] revision [newsletter-api-00012-kq4] has been deployed and is serving 100 percent of traffic.\nService URL: https://newsletter-api-x7k2.a.run.app',
      maskedNote: 'CloudBuddy hid the webhook secret in the command before saving it.',
      approval: ask('ap-n7', 'approved', "There's no Google Cloud adapter yet, and this command isn't clearly read-only.", {
        lossTitle: 'What will happen',
        loss: ['Restarts the signup API with new settings'],
        asked: '20:39:58', decided: '20:40:12',
      }),
    }),
    mk({
      id: 'n8', provider: 'Resend', product: 'Broadcasts', action: 'create', via: 'mcp', time: '20:41:30', ms: 500,
      title: 'Saved the launch email as a draft', resource: 'Broadcasts · bc_7e2a91f4',
      facts: [['To', 'Waitlist · 1,240 people'], ['Subject', 'Shipyard Weekly is here']],
      api: 'resend · create_broadcast',
      command: 'create_broadcast({\n  "audience_id": "aud_waitlist",\n  "from": "news@shipyard.dev",\n  "subject": "Shipyard Weekly is here"\n})',
      why: 'Lets you look at the email before it goes out.',
      output: '{\n  "id": "bc_7e2a91f4",\n  "status": "draft"\n}',
    }),
    mk({
      id: 'n9', provider: 'Resend', product: 'Broadcasts', action: 'send', via: 'mcp', status: 'waiting', tags: ['people', 'irreversible'], time: '20:41:36',
      title: 'Send the launch email to 1,240 people', resource: 'Broadcasts · bc_7e2a91f4',
      api: 'resend · send_broadcast', command: 'send_broadcast({ "id": "bc_7e2a91f4" })',
      why: 'You asked me to send the launch email once everything was set up. The domain is verified and the signup API is live.',
      approval: ask('ap-n9', 'pending', "It reaches real people, and an email can't be unsent.", {
        lossTitle: 'What will happen',
        loss: ['1,240 people on the Waitlist get this email', 'Sent from news@shipyard.dev', "Emails can't be unsent"],
        asked: '20:41:36',
      }),
    }),
  ]
  return conv(
    {
      id: 'news', title: 'Launch the paid newsletter', agent: 'claude', model: 'Opus 5.5', workdir: '~/code/shipyard-newsletter', projectId: 'newsletter',
      subtitle: '4 providers · Resend, Cloudflare, Stripe, Google Cloud', status: 'waiting',
    },
    steps,
    [
      ['user', 'Set up the paid newsletter. Send from news@shipyard.dev with Resend, add an $8/month plan in Stripe, and connect it to the signup API on Cloud Run. Then send the launch email to everyone on the waitlist.'],
      ['agent', "I'll start with email, since the domain has to be verified before anything can be sent."],
      ['steps', ['n1', 'n2']],
      ['agent', "Resend gave me 3 DNS records. I'll add them in Cloudflare, where `shipyard.dev` lives."],
      ['steps', ['l1', 'n3', 'n4']],
      ['agent', "The domain is verified. Next is the plan in Stripe. This runs in live mode, and a live price can't be edited after it's created, so I need your OK."],
      ['approval', 'n5'],
      ['steps', ['n5']],
      ['agent', "Next, a webhook so the signup API hears about new subscribers. It's live mode too, so it asks first."],
      ['approval', 'n6'],
      ['steps', ['n6', 'l2']],
      ['agent', 'Now I\'ll give the signup API the plan ID and the webhook secret. CloudBuddy has no Google Cloud adapter yet, so it asks before this change.'],
      ['approval', 'n7'],
      ['steps', ['n7', 'n8']],
      ['agent', "Everything is ready and the launch email is saved as a draft. Sending it reaches 1,240 people and can't be undone, so I need your OK."],
      ['approval', 'n9'],
    ],
  )
}

export function deployConv(): ConversationDetail {
  const aws = { provider: 'AWS' }
  const steps = [
    mk({
      id: 'd1', ...aws, product: 'Account', action: 'read', time: '20:12:41', ms: 600,
      title: 'Checked which AWS account is signed in', resource: 'arn:aws:iam::482193650217:user/cli-admin',
      facts: [['Account', '4821-9365-0217'], ['Profile', 'default']],
      api: 'sts:GetCallerIdentity', command: 'aws sts get-caller-identity',
      why: 'Make sure everything lands in the right account before changing anything.',
      output: '{\n  "UserId": "AIDA••••••••••••QX4",\n  "Account": "482193650217",\n  "Arn": "arn:aws:iam::482193650217:user/cli-admin"\n}',
      maskedNote: 'CloudBuddy masked part of an ID before saving this output.',
    }),
    mk({
      id: 'd2', ...aws, product: 'S3', action: 'read', time: '20:12:44', ms: 900,
      title: 'Listed your S3 buckets', resource: '4 buckets in this account',
      facts: [['Found', '4 buckets'], ['Old staging', 'shipyard-web-staging-old']],
      api: 's3:ListBuckets', command: 'aws s3api list-buckets --query "Buckets[].Name"',
      why: 'See what already exists, so the new bucket does not clash with an old name.',
      output: '[\n  "shipyard-logs",\n  "shipyard-uploads",\n  "shipyard-web-staging-old",\n  "cdk-hnb659fds-assets-482193650217-us-east-1"\n]',
    }),
    mk({
      id: 'd3', ...aws, product: 'S3', action: 'create', time: '20:13:02', ms: 1200,
      title: 'Created bucket shipyard-web-prod', resource: 'arn:aws:s3:::shipyard-web-prod',
      facts: [['Region', 'us-east-1'], ['Access', 'Private']],
      api: 's3:CreateBucket', command: 'aws s3api create-bucket --bucket shipyard-web-prod --region us-east-1',
      why: 'A new home for the built site.', output: '{\n  "Location": "/shipyard-web-prod"\n}',
    }),
    mk({
      id: 'd4', ...aws, product: 'S3', action: 'change', time: '20:13:04', ms: 700,
      title: 'Blocked public access to the bucket', resource: 'shipyard-web-prod',
      facts: [['Public access', 'Blocked (all 4 settings)']],
      uses: [{ fromStepId: 'd3', what: 'bucket' }],
      api: 's3:PutPublicAccessBlock',
      command: 'aws s3api put-public-access-block --bucket shipyard-web-prod --public-access-block-configuration BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true',
      why: 'Visitors will reach the site through CloudFront, so the bucket itself stays private.',
      output: '(no output: the command succeeded)',
    }),
    mk({
      id: 'dl1', kind: 'local', provider: 'Local', product: 'Shell', action: 'create', via: 'tool', time: '20:13:08', ms: 5200,
      title: 'Built the site', resource: './dist',
      api: 'shell · run', command: 'npm run build',
      why: 'The site has to be built before it can be uploaded.',
      output: 'vite v6 building for production...\n✓ 42 modules transformed.\ndist/index.html  0.46 kB',
    }),
    mk({
      id: 'd5', ...aws, product: 'S3', action: 'create', time: '20:13:19', ms: 6400,
      title: 'Uploaded the built site', resource: 's3://shipyard-web-prod/',
      facts: [['Files', '42'], ['Size', '3.1 MB']],
      uses: [{ fromStepId: 'd3', what: 'bucket' }],
      api: 's3:PutObject × 42', command: 'aws s3 sync ./dist s3://shipyard-web-prod',
      why: 'Copies the files in ./dist into the new bucket.',
      output: 'upload: dist/index.html to s3://shipyard-web-prod/index.html\nupload: dist/assets/app-4f9c1e.js to s3://shipyard-web-prod/assets/app-4f9c1e.js\n… 40 more files',
    }),
    mk({
      id: 'd6', ...aws, product: 'CloudFront', action: 'create', time: '20:13:41', ms: 3800,
      title: 'Created a CloudFront distribution', resource: 'E2QW8XK4N1ZP7B',
      facts: [['Address', 'd3k9x7f2m1q8.cloudfront.net'], ['Reads from', 'shipyard-web-prod']],
      api: 'cloudfront:CreateDistribution', command: 'aws cloudfront create-distribution --distribution-config file://cloudfront-config.json',
      why: 'Serves the site over HTTPS from locations close to your visitors.',
      output: '{\n  "Distribution": {\n    "Id": "E2QW8XK4N1ZP7B",\n    "DomainName": "d3k9x7f2m1q8.cloudfront.net",\n    "Status": "InProgress"\n  }\n}',
    }),
    mk({
      id: 'd7', ...aws, product: 'S3', action: 'change', time: '20:13:58', ms: 800,
      title: 'Let CloudFront read the bucket', resource: 'shipyard-web-prod · bucket policy',
      facts: [['Allows', 's3:GetObject'], ['Only for', 'E2QW8XK4N1ZP7B']],
      uses: [{ fromStepId: 'd6', what: 'distribution ID' }],
      api: 's3:PutBucketPolicy', command: 'aws s3api put-bucket-policy --bucket shipyard-web-prod --policy file://bucket-policy.json',
      why: 'The bucket is private, so CloudFront needs permission to read the files. Nobody else gets access.',
      output: '(no output: the command succeeded)',
    }),
    mk({
      id: 'd8', ...aws, product: 'S3', action: 'delete', tags: ['irreversible'], time: '20:15:02', ms: 3900,
      title: 'Delete bucket shipyard-web-staging-old', resource: 'arn:aws:s3:::shipyard-web-staging-old',
      uses: [{ fromStepId: 'd2', what: 'bucket' }],
      api: 's3:DeleteObject × 318, then s3:DeleteBucket', command: 'aws s3 rb s3://shipyard-web-staging-old --force',
      why: 'You asked me to clean up the old staging bucket. I checked: no CloudFront distribution or DNS record points to it.',
      output: 'delete: s3://shipyard-web-staging-old/index.html\n… 317 more files\nremove_bucket: shipyard-web-staging-old',
      approval: ask('ap-d8', 'approved', "It's a delete, and with versioning off the files can't be recovered.", {
        loss: ['318 files, 1.2 GB', 'Last changed Jul 23, 2026 (95 days ago)', "Versioning is off, so the files can't be recovered"],
        asked: '20:14:40', decided: '20:15:02',
      }),
    }),
    mk({
      id: 'd9', ...aws, product: 'CloudFront', action: 'read', time: '20:15:21', ms: 1100,
      title: 'Checked the site is live', resource: 'd3k9x7f2m1q8.cloudfront.net',
      facts: [['Status', 'Deployed']],
      uses: [{ fromStepId: 'd6', what: 'distribution ID' }],
      api: 'cloudfront:GetDistribution', command: 'aws cloudfront get-distribution --id E2QW8XK4N1ZP7B --query Distribution.Status',
      why: 'Confirms the new site is answering before I call it done.', output: '"Deployed"',
    }),
  ]
  return conv(
    {
      id: 'deploy', title: 'Deploy shop site to S3 + CloudFront', agent: 'claude', model: 'Opus 5.5', workdir: '~/code/shipyard-web', projectId: 'shop',
      extraFolders: ['~/code/shipyard-infra'],
      subtitle: 'AWS · us-east-1 · account 4821-9365-0217', status: 'done',
      result: { title: 'Your site is live', line: 'd3k9x7f2m1q8.cloudfront.net', at: at('20:15:21') },
    },
    steps,
    [
      ['user', "Deploy the site in ./dist to AWS. Put it on S3 with CloudFront in front so it loads fast, and clean up the old staging bucket while you're at it."],
      ['agent', "I'll check which account you're signed in to and what's already there before I change anything."],
      ['steps', ['d1', 'd2']],
      ['agent', "I found the old bucket, `shipyard-web-staging-old`. I'll set up the new site first and clean up at the end."],
      ['steps', ['d3', 'd4', 'dl1', 'd5', 'd6', 'd7']],
      ['agent', "The new site is set up. Deleting the old staging bucket removes 318 files and can't be undone, so I need your OK."],
      ['approval', 'd8'],
      ['steps', ['d8', 'd9']],
      ['agent', 'Done. Your site is live at `d3k9x7f2m1q8.cloudfront.net`, and the old staging bucket is gone.'],
    ],
  )
}

export function alarmConv(): ConversationDetail {
  const aws = { provider: 'AWS' }
  const steps = [
    mk({
      id: 'a1', ...aws, product: 'Account', action: 'read', day: 1, time: '09:31:10', ms: 500,
      title: 'Checked which AWS account is signed in', resource: 'arn:aws:iam::482193650217:user/cli-admin',
      facts: [['Account', '4821-9365-0217']], api: 'sts:GetCallerIdentity', command: 'aws sts get-caller-identity',
      why: 'Make sure the alarm goes in the right account.',
      output: '{\n  "Account": "482193650217",\n  "Arn": "arn:aws:iam::482193650217:user/cli-admin"\n}',
    }),
    mk({
      id: 'a2', ...aws, product: 'CloudWatch', action: 'read', day: 1, time: '09:31:15', ms: 800,
      title: 'Looked for existing billing alarms', resource: 'us-east-1 · names starting "billing"',
      facts: [['Found', 'None']], api: 'cloudwatch:DescribeAlarms',
      command: 'aws cloudwatch describe-alarms --alarm-name-prefix billing --region us-east-1',
      why: "Billing data only lives in us-east-1, and I didn't want to make a duplicate.",
      output: '{\n  "MetricAlarms": [],\n  "CompositeAlarms": []\n}',
    }),
    mk({
      id: 'al1', kind: 'local', provider: 'Local', product: 'Shell', action: 'read', via: 'tool', day: 1, time: '09:31:20', ms: 90,
      title: 'Searched the project for the ops address', resource: 'ops@shipyard.dev',
      api: 'shell · run', command: 'grep -r "ops@" infra/',
      why: 'To reuse the address the project already has.', output: 'infra/contacts.md: ops@shipyard.dev',
    }),
    mk({
      id: 'a3', ...aws, product: 'SNS', action: 'create', day: 1, time: '09:31:29', ms: 900,
      title: 'Created alert topic billing-alerts', resource: 'arn:aws:sns:us-east-1:482193650217:billing-alerts',
      facts: [['Region', 'us-east-1']], api: 'sns:CreateTopic',
      command: 'aws sns create-topic --name billing-alerts --region us-east-1',
      why: 'The alarm needs somewhere to send its message.',
      output: '{\n  "TopicArn": "arn:aws:sns:us-east-1:482193650217:billing-alerts"\n}',
    }),
    mk({
      id: 'a4', ...aws, product: 'SNS', action: 'create', day: 1, time: '09:31:33', ms: 700,
      title: 'Subscribed ops@shipyard.dev to the topic', resource: 'email · ops@shipyard.dev',
      facts: [['Status', 'Waiting for email confirmation']], api: 'sns:Subscribe',
      command: 'aws sns subscribe --topic-arn arn:aws:sns:us-east-1:482193650217:billing-alerts --protocol email --notification-endpoint ops@shipyard.dev',
      why: 'This is the address you gave me.', output: '{\n  "SubscriptionArn": "pending confirmation"\n}',
    }),
    mk({
      id: 'a5', ...aws, product: 'CloudWatch', action: 'create', day: 1, time: '09:31:48', ms: 1100,
      title: 'Created alarm for a bill over $50', resource: 'billing-over-50',
      facts: [['Watches', 'EstimatedCharges (USD)'], ['Fires above', '$50.00'], ['Checks every', '6 hours']],
      uses: [{ fromStepId: 'a3', what: 'alert topic' }],
      api: 'cloudwatch:PutMetricAlarm',
      command: 'aws cloudwatch put-metric-alarm --alarm-name billing-over-50 --namespace AWS/Billing --metric-name EstimatedCharges --dimensions Name=Currency,Value=USD --statistic Maximum --period 21600 --evaluation-periods 1 --threshold 50 --comparison-operator GreaterThanThreshold --alarm-actions arn:aws:sns:us-east-1:482193650217:billing-alerts --region us-east-1',
      why: "Sends the email the first time this month's estimated bill goes over $50.",
      output: '(no output: the command succeeded)',
    }),
  ]
  return conv(
    {
      id: 'alarm', title: 'Set up a monthly billing alarm', agent: 'codex', model: '', workdir: '~/code/shipyard-infra',
      subtitle: 'AWS · us-east-1 · account 4821-9365-0217', status: 'done',
      result: { title: 'Billing alarm is on', line: 'Emails ops@shipyard.dev above $50', at: at('09:31:48', 1) },
    },
    steps,
    [
      ['user', 'Email ops@shipyard.dev if our AWS bill goes over $50 this month.'],
      ['agent', "I'll check the account and look for an existing billing alarm first."],
      ['steps', ['a1', 'a2', 'al1']],
      ['agent', "There's no billing alarm yet. I'll create an alert topic, subscribe your address, then add the alarm."],
      ['steps', ['a3', 'a4', 'a5']],
      ['agent', "Done. Check ops@shipyard.dev for a confirmation email from AWS and click the link, or the alert can't reach you."],
    ],
  )
}

export function ec2Conv(): ConversationDetail {
  const aws = { provider: 'AWS', product: 'EC2' }
  const steps = [
    mk({
      id: 'e1', ...aws, action: 'read', day: 4, time: '14:06:12', ms: 1300,
      title: 'Found 6 test servers', resource: 'tag env=test',
      facts: [['Running', '2'], ['Stopped', '4']], api: 'ec2:DescribeInstances',
      command: 'aws ec2 describe-instances --filters Name=tag:env,Values=test',
      why: 'Start from the servers you tagged as test.',
      output: 'i-0a7f3c91e2b4d5f68  stopped  test-api-old\ni-0c19e7a4b2f6d8e31  stopped  test-db-2\ni-05d2b8f1a9c7e3a40  running  test-api\n… 3 more',
    }),
    mk({
      id: 'e2', provider: 'AWS', product: 'CloudWatch', action: 'read', day: 4, time: '14:06:40', ms: 2200,
      title: 'Checked when each server was last busy', resource: 'CPU use, last 30 days',
      facts: [['Idle 30+ days', '2 servers']], api: 'cloudwatch:GetMetricStatistics',
      command: 'aws cloudwatch get-metric-statistics --namespace AWS/EC2 --metric-name CPUUtilization --dimensions Name=InstanceId,Value=i-0a7f3c91e2b4d5f68 --start-time 2026-09-22T00:00:00Z --end-time 2026-10-22T00:00:00Z --period 86400 --statistics Maximum',
      why: 'Only servers nobody has used in a month should go.',
      output: '{\n  "Label": "CPUUtilization",\n  "Datapoints": []\n}',
    }),
    mk({
      id: 'el1', kind: 'local', provider: 'Local', product: 'Files', action: 'create', via: 'file', day: 4, time: '14:07:20', ms: 35,
      title: 'Wrote the cleanup plan', resource: 'cleanup-notes.md',
      api: 'file · write', command: 'write cleanup-notes.md',
      why: 'A short record of what will be removed and why.',
    }),
    mk({
      id: 'e3', ...aws, action: 'create', day: 4, time: '14:07:55', ms: 1600,
      title: "Saved a backup of test-api-old's disk", resource: 'snap-0e7a24c9b1d3f5a86',
      facts: [['From disk', 'vol-0c41f9e2a7b3d5e10'], ['Size', '30 GB']], api: 'ec2:CreateSnapshot',
      command: 'aws ec2 create-snapshot --volume-id vol-0c41f9e2a7b3d5e10 --description "Backup before deleting test-api-old"',
      why: 'A safety copy in case you need anything from it later.',
      output: '{\n  "SnapshotId": "snap-0e7a24c9b1d3f5a86",\n  "State": "pending"\n}',
    }),
    mk({
      id: 'e4', ...aws, action: 'delete', tags: ['irreversible'], day: 4, time: '14:08:51', ms: 2000,
      title: 'Terminate server test-api-old', resource: 'i-0a7f3c91e2b4d5f68',
      uses: [{ fromStepId: 'e1', what: 'server ID' }],
      api: 'ec2:TerminateInstances', command: 'aws ec2 terminate-instances --instance-ids i-0a7f3c91e2b4d5f68',
      why: 'Unused for over a month, and its disk is backed up.',
      output: '{\n  "TerminatingInstances": [{\n    "InstanceId": "i-0a7f3c91e2b4d5f68",\n    "CurrentState": { "Name": "shutting-down" }\n  }]\n}',
      approval: ask('ap-e4', 'approved', "It's a delete. A terminated server can't be started again.", {
        loss: ['t3.medium, stopped for 41 days', 'Its disk is backed up in snap-0e7a24c9b1d3f5a86'],
        asked: '14:08:30', decided: '14:08:51', day: 4,
      }),
    }),
    mk({
      id: 'e5', ...aws, action: 'delete', status: 'denied', tags: ['irreversible'], day: 4, time: '14:09:30',
      title: 'Terminate server test-db-2', resource: 'i-0c19e7a4b2f6d8e31',
      uses: [{ fromStepId: 'e1', what: 'server ID' }],
      api: 'ec2:TerminateInstances', command: 'aws ec2 terminate-instances --instance-ids i-0c19e7a4b2f6d8e31',
      why: 'Unused for over a month.',
      approval: ask('ap-e5', 'denied', "It's a delete, and its disk has no backup.", {
        loss: ['100 GB data disk with no backup', 'Stopped for 33 days'],
        asked: '14:09:10', decided: '14:09:30', day: 4,
      }),
    }),
    mk({
      id: 'e6', ...aws, action: 'delete', day: 4, time: '14:10:02', ms: 600,
      title: 'Delete security group test-legacy', resource: 'sg-04b2e9d1c3a5f7e80',
      api: 'ec2:DeleteSecurityGroup', command: 'aws ec2 delete-security-group --group-id sg-04b2e9d1c3a5f7e80',
      why: 'Its only server was test-api-old.', output: '(no output: the command succeeded)',
      approval: ask('ap-e6', 'approved', 'Deletes always ask.', {
        loss: ['No servers use it anymore'], asked: '14:09:50', decided: '14:10:02', day: 4,
      }),
    }),
  ]
  return conv(
    {
      id: 'ec2', title: 'Remove unused EC2 test servers', agent: 'claude', model: 'Opus 5.5', workdir: '~/code/shipyard-infra',
      subtitle: 'AWS · us-east-1 · account 4821-9365-0217', status: 'done',
      result: { title: 'Cleanup finished', line: '1 server removed · 1 kept by you', at: at('14:10:02', 4) },
    },
    steps,
    [
      ['user', 'Clean up our EC2 test servers. Anything nobody has used in a month can go.'],
      ['agent', "I'll list the test servers and check how busy each one has been."],
      ['steps', ['e1', 'e2', 'el1']],
      ['agent', "Two servers have been idle for over a month: `test-api-old` and `test-db-2`. I'll back up test-api-old's disk first, then ask before each delete."],
      ['steps', ['e3']],
      ['approval', 'e4'],
      ['approval', 'e5'],
      ['agent', 'Okay, keeping `test-db-2`.'],
      ['approval', 'e6'],
      ['agent', 'Done. `test-api-old` is gone and its disk is backed up. `test-db-2` is still there, and the unused security group is deleted.'],
    ],
  )
}

export function seedConversations(): ConversationDetail[] {
  return [newsletterConv(), deployConv(), alarmConv(), ec2Conv()]
}

export function seedProjects(): Project[] {
  const updatedAt = new Date().toISOString()
  return [
    { id: 'newsletter', name: 'Newsletter', folders: ['~/code/shipyard-newsletter'], chatCount: 0, updatedAt },
    { id: 'shop', name: 'Shipyard shop', folders: ['~/code/shipyard-web', '~/code/shipyard-infra'], chatCount: 0, updatedAt },
  ]
}

export function seedSettings(): Settings {
  return { agent: 'claude', folder: '~/code/shipyard-infra' }
}

export function seedConnections(): ConnectionsResponse {
  const conn = (c: Omit<Connection, 'supported'> & { supported?: boolean }): Connection => ({ supported: true, ...c })
  return {
    scannedAt: new Date().toISOString(),
    cli: [
      conn({ id: 'aws', kind: 'cli', mark: 'AW', name: 'AWS', how: 'aws 2.27.50', account: 'Account 4821-9365-0217', detail: 'profile default · us-east-1', adapter: 'builtin', adapterNote: 'One adapter covers every AWS service.', rule: 'irreversible' }),
      conn({ id: 'gcloud', kind: 'cli', mark: 'GC', name: 'Google Cloud', how: 'gcloud 541.0.0', account: 'ops@shipyard.dev', detail: 'project shipyard-prod', adapter: 'generic', rule: 'irreversible' }),
      conn({ id: 'stripe-cli', kind: 'cli', mark: 'St', name: 'Stripe', how: 'stripe 1.31.0', account: 'Shipyard Inc', detail: 'test and live keys', adapter: 'builtin', adapterNote: 'Knows live mode, refunds and payouts.', rule: 'irreversible' }),
      conn({ id: 'cloudflare', kind: 'cli', mark: 'Cf', name: 'Cloudflare', how: 'wrangler 4.45.0 · API token', account: 'Shipyard', detail: 'zone shipyard.dev', adapter: 'builtin', rule: 'irreversible' }),
      conn({ id: 'vercel', kind: 'cli', mark: 'Ve', name: 'Vercel', how: 'vercel 48.2.0', account: 'team shipyard', detail: '2 projects', adapter: 'generated', adapterNote: 'Built from its API spec.', rule: 'irreversible' }),
      conn({ id: 'gh', kind: 'cli', mark: 'Gh', name: 'GitHub', how: 'gh 2.81.0', account: 'shipyard-bot', detail: 'org shipyard', adapter: 'generated', adapterNote: 'Built from its command list.', rule: 'readonly' }),
    ],
    mcp: [
      conn({ id: 'resend', kind: 'mcp', mark: 'Re', name: 'Resend', how: 'MCP · local · 7 tools', account: 'Shipyard team', detail: 'set up in Claude Code and Codex', adapter: 'builtin', adapterNote: 'Knows which tools send email.', rule: 'irreversible' }),
      conn({ id: 'stripe-mcp', kind: 'mcp', mark: 'St', name: 'Stripe', how: 'MCP · mcp.stripe.com · 24 tools', account: 'Shipyard Inc', detail: 'set up in Claude Code', adapter: 'builtin', rule: 'live' }),
      conn({ id: 'supabase', kind: 'mcp', mark: 'Sb', name: 'Supabase', how: 'MCP · local · 19 tools', account: 'project shipyard-db', detail: 'set up in Claude Code', adapter: 'hints', adapterNote: '16 of 19 tools say whether they read or destroy.', rule: 'off' }),
      conn({ id: 'linear', kind: 'mcp', mark: 'Li', name: 'Linear', how: 'MCP · mcp.linear.app · OAuth', account: 'Not signed in here', detail: 'needs a browser sign-in', adapter: 'generic', adapterNote: "Not supported yet: CloudBuddy can't sign in to OAuth servers.", rule: 'irreversible', supported: false }),
    ],
  }
}
