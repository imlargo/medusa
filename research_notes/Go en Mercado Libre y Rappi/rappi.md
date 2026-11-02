# Rappi backend platform and its use of Go (research as of 2026-11-01)

Note on sourcing: Rappi publishes very little about its backend. Its engineering blog (engineering.rappi.com / medium.com/rappitech) returned HTTP 403 to automated fetches, and search found no Go-specific post on it. Most of the hard facts below come from **vendor case studies** (KrakenD, Istio, AWS), **job postings** (aggregators, mostly undated or "6+ months ago"), and **2026 Spanish- and Portuguese-language press** repeating a joint Rappi/AWS announcement. Treat vendor and press figures as company-reported, not audited.

## 1. Tech stack and how much Go (vs Kotlin/Java, Python, Node, PHP)

### Takeaway
Rappi is a polyglot shop where teams choose their own language. In job postings Go is the most visible backend language, with Kotlin/Java (Spring Boot, Micronaut) as the other main option, plus Node.js and Python. No public source gives a share of services by language, and no source documents a migration to Go or the language of the original monolith.

### Cited Findings
- KrakenD case study (vendor, undated; describes events from 2020-2021): "750+ developers who can choose their coding language", with a ~70-person Cloud Engineering (Infra) team providing the tooling. — [KrakenD case study](https://www.krakend.io/case-study/rappi/)
- An Engineering Lead posting lists the main technologies as Node.js, Kotlin, Golang and Java, on a microservices architecture with Postgres, Redis and Kafka (aggregator, undated). — [startup.jobs](https://startup.jobs/engineering-lead-rappi-rappi-1658078); [Foundation Capital jobs](https://jobs.foundationcapital.com/companies/rappi-2/jobs/32417984-engineer-lead)
- A "Senior Backend Developer - Go" posting (Rappi Technology Colombia, "posted 6+ months ago", closed) asks for Go, asynchronous programming, Kafka, SQS, and monitoring with New Relic or Splunk. It also asks for Docker, Kubernetes, microservices, CI/CD, REST and SQL/NoSQL. It names no Go frameworks. — [Foundation Capital jobs](https://jobs.foundationcapital.com/companies/rappi-2/jobs/30739533-senior-backend-developer-go)
- A "Software Engineer Backend III" posting ("6+ months ago", closed) lists Go, Java, Kotlin, Python and Bash; Spring Boot; PostgreSQL, MySQL, MongoDB, Cassandra and Redis; Kafka and RabbitMQ; REST and gRPC; Docker/Kubernetes; Terraform (preferred) and Ansible; and Jenkins, GitHub Actions, GitLab CI and CircleCI. — [Foundation Capital jobs](https://jobs.foundationcapital.com/companies/rappi-2/jobs/65326589-software-engineer-backend-iii)
- Other Backend III postings (summarized from search snippets) describe building microservices "in Go or Kotlin", with "advanced Go (highly preferred) or Kotlin (JVM)" and "4+ years of backend development, ideally with Golang". — [Built In](https://builtin.com/job/software-engineer-backend-iii/8276891); [Foundation Capital jobs](https://jobs.foundationcapital.com/companies/rappi-2/jobs/65841576-software-engineer-backend-iii)
- A "Backend Developer Java/Kotlin" posting (summarized from search snippets) mentions Kotlin/Java with Spring and Micronaut, and Go with Gin and Echo. — [Foundation Capital jobs](https://jobs.foundationcapital.com/companies/rappi-2/jobs/29900067-backend-developer-java-kotlin); [Soma Capital jobs](https://jobs.somacap.com/companies/rappi/jobs/28800447-backend-developer-java-kotlin)
- A RemoteOK listing for "Backend Developer Golang Shop" in Bogotá (about 4 years old per search snippet, so roughly 2022) shows Go in use in the Shop/commerce domain. — [RemoteOK](https://remoteok.com/remote-jobs/111181-remote-backend-developer-golang-shop-rappi)

### Inferences
- Over several years of postings, Go keeps appearing as the "preferred" or first-listed backend language, and Kotlin as the JVM alternative. That points to Go plus Kotlin as the current default pair for new services, with Java/Spring, Node and Python as present but secondary. This is inferred from hiring patterns, not stated by Rappi.
- Gin and Echo are the only Go web frameworks named anywhere. Nothing suggests a single internal Go framework is mandated, which fits the "choose your language" policy.

### Gaps
- No source gives the share of services or code by language.
- I found no public source on the original monolith's language. Searches for a PHP/Laravel legacy returned nothing about Rappi, so the PHP premise is unconfirmed.
- I found no post or talk explaining why Rappi adopted Go.

## 2. Architecture: microservices, containers, gateway, events, data, payments, peaks

### Takeaway
The platform is very fine-grained microservices ("nanoservices", 2,000+ plus 1,500 for RappiPay per KrakenD) on AWS. It runs on both ECS and Kubernetes (EKS, 50+ clusters per the Istio case study), behind 20+ KrakenD gateways for JWT validation, with an Istio/Envoy mesh. Kafka and SQS (plus RabbitMQ) carry asynchronous traffic. MongoDB-compatible DocumentDB (one cluster per line of business per country), PostgreSQL/MySQL and Redis are the data layer. In 2026 about 95% of infrastructure ran on AWS.

### Cited Findings
**Compute and cloud**
- Rappi is among the top 100 AWS resource consumers worldwide, with 7,000+ EC2 servers and 1,800+ EKS nodes, on a dual ECS and Kubernetes stack (vendor, undated). — [KrakenD case study](https://www.krakend.io/case-study/rappi/)
- 2,000+ microservices on ECS, described as "nanoservices", plus 1,500 more for Rappi Pay. The monolith was "broken up in 2021", and developers used the slogan "I killed the monolith". — [KrakenD case study](https://www.krakend.io/case-study/rappi/)
- Istio case study (undated, roughly 2022 based on the $8B valuation it cites): 50+ Kubernetes clusters, the largest with 20,000+ containers and 30,000+ containers in total; 1,500+ deployments per day; Istio manages traffic between 1,500+ applications on the most critical clusters; adoption grew from a team of 30 to 1,500+ developers. — [Istio case study](https://istio.io/latest/about/case-studies/rappi/)
- May 2026 press (Rappi/AWS announcement): about 95% of infrastructure on AWS; cost per order down 70%; order-processing capacity up 30%; Graviton adopted "in the past year" for about 15% more efficiency; 20+ AWS security services; about 1M attack attempts per week in Colombia alone. — [Mercado (AR), 2026-05-06](https://mercado.com.ar/ruta-digital/rappi-concentra-95-de-su-infraestructura-en-aws-y-reduce-70-costos); [Valora Analitik, 2026-05-10](https://www.valoraanalitik.com/?p=578945); [Baguete (BR)](https://www.baguete.com.br/noticias/rappi-tem-95-da-operacao-na-nuvem-da-aws)
- AWS has published Rappi FinOps material claiming about a 90% improvement in its benefit-cost ratio. The official AWS page did not render the Rappi content when fetched, so this rests on the search snippet and the casestudies.com mirror. — [casestudies.com mirror](https://www.casestudies.com/company/amazonwebservices/case-study/rappi-has-optimized-its-benefit-cost-ratio-by-90-in-the-aws-cloud-thanks-to-finops); [AWS innovators page](https://aws.amazon.com/solutions/case-studies/innovators/rappi)
- Rappi was named as a success case at AWS re:Invent 2022. — [La República](https://www.larepublica.co/empresas/en-re-invent-2022-encuentro-principal-de-aws-se-resalto-a-rappi-como-caso-de-exito-3498836); [Valora Analitik](https://www.valoraanalitik.com/ceo-de-aws-lanza-mas-servicios-en-la-nube-y-cita-a-rappi/)

**API gateway and service mesh**
- KrakenD (written in Go) was adopted in May 2020, with full implementation in about 1 month. It is the front JWT validator for all services, with Auth0 as the identity provider. There are 20+ gateways, each running in multiple containers and grouped by function (for example an "Orders gateway"), under the shared domain services.rappi.com behind a router. Metrics go to Splunk SignalFx. Configuration is generated from a parameters YAML plus a Jinja template of 35,000+ lines, wired into CI. Rappi's Nicolás Vila is quoted: "We didn't have to fight". — [KrakenD case study](https://www.krakend.io/case-study/rappi/)
- Istio replaced an in-house service mesh that had become too costly to maintain. It is used for per-service rate limits, circuit breakers, connection pools and timeouts, and for traffic segmentation and restricted endpoint communication for compliance. Quoted: Juan Franco Cusa (DevOps Tech Lead), "Istio gives us the flexibility to set different specs for different services". The stated next step was multi-cluster mesh. — [Istio case study](https://istio.io/latest/about/case-studies/rappi/)

**Events and messaging**
- Postings name Kafka and SQS for service-to-service and external messaging, Kafka and RabbitMQ as event brokers, and Pub/Sub in some listings. — [Senior Backend Go posting](https://jobs.foundationcapital.com/companies/rappi-2/jobs/30739533-senior-backend-developer-go); [Backend III posting](https://jobs.foundationcapital.com/companies/rappi-2/jobs/65326589-software-engineer-backend-iii)
- A third-party blog (4Geeks) describes a Kafka event-driven architecture, "1M orders/day" and multilevel caching. It cites no primary source, so low reliability. — [4Geeks blog](https://blog.4geeks.io/es/44-arquitectura-de-rappi-escalando-a-1m-de-pedidos/)

**Databases**
- DocumentDB (AWS customer story): 100 MongoDB databases migrated to Amazon DocumentDB in 3 months; the largest cluster (5 instances) took 2 weeks; one cluster per line of business (9) per country, managed by 2 engineers. The workloads had earlier moved from Elasticsearch to document stores. On compatibility: "we needed to move microservices that were already working... without changing much code" (Soto, Rappi). AWS slides claim 55% fewer instances, 16x lower latency on ranking queries via read replicas, and 60% FTE cost savings. — [AWS customer story](https://go.aws/3orujSn); [casestudies.com mirror](https://www.casestudies.com/company/amazonwebservices/case-study/rappi-improves-efficiency-stability-and-speed-with-amazon-documentdb-with-mongodb-compatibility); [AWS DocumentDB deep-dive deck](https://pages.awscloud.com/rs/112-TZM-766/images/DBS.D4S2_Amazon_DocumentDB_Deep_Dive_v2.pdf)
- Postings list PostgreSQL, MySQL, MongoDB, Cassandra and Redis (for caching). — [Backend III posting](https://jobs.foundationcapital.com/companies/rappi-2/jobs/65326589-software-engineer-backend-iii)
- Rappi built a retail personalization engine with Amazon Personalize (AWS story). An AWS Bogotá Summit session (DAT301) covers Rappi's use of graph databases, but the PDF could not be text-extracted. — [casestudies.com](https://www.casestudies.com/company/amazonwebservices/case-study/how-on-demand-delivery-company-rappi-built-an-advanced-retail-personalization-engine-using-amazon-personalize); [AWS Summit Bogotá DAT301 PDF](https://d1.awsstatic.com/events/Summits/awsbogotasummit/Ent%C3%A9rese_c%C3%B3mo_Rappi_utiliza_grafos_para_analizar_las_relaciones_entre_los_datos_mejorando_el_entendimiento_de_sus_clientes_DAT301.pdf)

**Products, payments and peaks**
- RappiPay grants credit "within minutes" using cloud data processing. Rappi Turbo's improved cloud geolocation cut its costs by more than 55%. Amazon Connect is being rolled out for customer service (2026). — [Valora Analitik, 2026-05-10](https://www.valoraanalitik.com/?p=578945); [Central do Varejo](https://centraldovarejo.com.br/?p=40398)
- Rappi's dark stores, smart routing and Turbo engine reportedly power Amazon Now ultra-fast delivery in Latin America (2026 press). — [Forbes México](https://forbes.com.mx/?p=1355165)
- Black Friday: 2024 projection of about 50% more orders; historical Black Friday uplift about 45%; Brazil 2020, orders up 40% year over year and a record. — [La República](https://www.larepublica.co/empresas/black-friday-en-rappi-4003966); [Vanguardia, 2024-11-27](https://www.vanguardia.com/entretenimiento/tendencias/2024/11/27/durante-black-friday-rappi-proyecta-un-crecimiento-de-50-en-el-numero-de-pedidos/); [Mercado & Consumo, 2020-12-03](https://mercadoeconsumo.com.br/03/12/2020/noticias/numero-de-pedidos-no-rappi-cresce-40-na-black-friday-e-bate-recorde/)

### Inferences
- The per-country, per-line-of-business DocumentDB layout points to multi-country isolation by sharding infrastructure per country rather than one global data plane. This is a reasonable reading but not stated as a general policy.
- Rappi pushes cross-cutting concerns (auth/JWT, rate limits, timeouts, circuit breakers, metrics) into platform layers (KrakenD, Istio) instead of per-service libraries. With teams free to choose languages, this keeps Go services thin.
- There is a minor dating tension. KrakenD says it was adopted in May 2020 and that the monolith was "broken up in 2021". The gateway likely came first, as a strangler-pattern front for the decomposition.

### Gaps
- No public detail on real-time courier tracking, dispatch or search (for example Elasticsearch/OpenSearch use today), or on RappiPay's internal architecture (only the 1,500-service count).
- No public detail on Black Friday capacity planning, load testing or autoscaling.
- DynamoDB and SNS are not mentioned in any Rappi source found.
- I found no requests-per-second figure.

## 3. Internal platform and developer experience, CI/CD, observability, SRE

### Takeaway
A central Cloud Engineering/DevOps group (about 70 people per KrakenD) runs a self-service platform. It built an in-house API on top of Kubernetes that creates traffic resources for each microservice automatically at deploy time, an automated Kubernetes plus Istio cluster provisioner, and templated gateway config in CI. Observability evidence points to Splunk/SignalFx and New Relic. Nothing public mentions Datadog or OpenTelemetry.

### Cited Findings
- Rappi built automated, production-ready Kubernetes plus Istio cluster deployment, and an in-house API layered over Kubernetes that manages microservices across clusters, creating each service's traffic resources automatically at deployment. — [Istio case study](https://istio.io/latest/about/case-studies/rappi/)
- When the previous monitoring solution hit its limit (past 30,000 containers), Rappi refactored its monitoring stack using Istio. Ezequiel Arielli (Senior DevOps Engineer): "Istio seamlessly gave our team the ability to monitor services at any scale". — [Istio case study](https://istio.io/latest/about/case-studies/rappi/)
- KrakenD metrics go to Splunk SignalFx, and gateway config comes from a YAML plus Jinja template integrated with Rappi's CI tools. — [KrakenD case study](https://www.krakend.io/case-study/rappi/)
- Job postings name New Relic or Splunk for monitoring, Terraform (preferred) and Ansible for IaC, and a range of CI systems (Jenkins, GitHub Actions, GitLab CI, CircleCI). — [Senior Backend Go posting](https://jobs.foundationcapital.com/companies/rappi-2/jobs/30739533-senior-backend-developer-go); [Backend III posting](https://jobs.foundationcapital.com/companies/rappi-2/jobs/65326589-software-engineer-backend-iii)
- 1,500+ deployments per day (Istio case study, roughly 2022). — [Istio case study](https://istio.io/latest/about/case-studies/rappi/)
- The engineering blog publishes leadership and role posts (for example "Hi! I'm a VP of Engineering at Rappi" by Facundo Viale, and "Leading an Engineering team at Rappi", Dec 2021). Content not retrievable (403). — [engineering.rappi.com](https://engineering.rappi.com/); [Medium Rappi Tech](https://medium.com/rappitech)
- A YouTube video, "Charlando con el equipo de Infraestructura de RAPPI", exists. Not reviewed. — [YouTube](https://www.youtube.com/watch?v=2BKJrtVZHqk)

### Inferences
- The CI list in postings is generic across many companies' templates. It probably does not reflect Rappi's actual CI system, so don't rely on it.

### Gaps
- No public info on service templates or scaffolding, an internal Go chassis, coding standards, SLO practices, on-call or incident process, or tracing tools.
- No evidence of Datadog or OpenTelemetry at Rappi.

## 4. Go-specific: open source, internal libraries, talks

### Takeaway
I found no official Rappi open-source Go code, no public internal Go library, no Go coding guide and no Go talk by Rappi engineers. Rappi's largest visible Go dependency is third-party infrastructure written in Go (KrakenD; Kubernetes/Istio control planes).

### Cited Findings
- A search for github.com/rappi Go repositories surfaced only unrelated projects and a hobby crawler ("goashot") by a non-Rappi author. — [awesome.ecosyste.ms goashot](https://awesome.ecosyste.ms/projects/github.com%2Fmarianozunino%2Fgoashot)
- Bogotá Go meetups (Golang Bogota Developers, Golang en español, BoGOLang) show no documented Rappi talk. An "El caso de éxito de Rappi" event on 2016-08-24 was at GDG/Android Bogotá, with no evidence it covered Go. — [Golang Bogota Developers](https://www.meetup.com/es-es/golang-bogota-developers/); [BoGOLang on X](https://x.com/BoGOLang); [GDG Bogotá event](https://www.meetup.com/en-AU/gdgbogota/events/233446606/attendees)
- The only Go web frameworks named in Rappi material are Gin and Echo (posting snippet). — [Foundation Capital jobs](https://jobs.foundationcapital.com/companies/rappi-2/jobs/29900067-backend-developer-java-kotlin)

### Inferences
- Rappi seems to treat Go as a hiring and implementation choice, not something to publish about. For Go-specific practice lessons, Mercado Libre is the far richer public source.

### Gaps
- No GopherCon Latam or other Go conference talk by a Rappi engineer was found. A manual check of GopherCon Latam and Golang Colombia YouTube channels could change this.
- Internal Go libraries and standards are not public.

## 5. Scale numbers (dated)

### Takeaway
The figures are scattered and mostly company-reported via vendors and press. The most recent are from May 2026.

### Cited Findings
| Metric | Value | Date | Source type | Source |
|---|---|---|---|---|
| Orders | 28M+ per month, 9 countries | May 2026 | Press (Rappi/AWS) | [Valora Analitik](https://www.valoraanalitik.com/?p=578945) |
| Infrastructure on AWS | ~95% | May 2026 | Press | [Mercado](https://mercado.com.ar/ruta-digital/rappi-concentra-95-de-su-infraestructura-en-aws-y-reduce-70-costos) |
| Cost per order | -70% | May 2026 | Press | same |
| Microservices | 2,000+ on ECS plus 1,500 RappiPay | ~2020-2022 (undated) | Vendor (KrakenD) | [KrakenD](https://www.krakend.io/case-study/rappi/) |
| Compute | 7,000+ EC2 servers, 1,800+ EKS nodes | undated | Vendor | same |
| Developers | 750+ (KrakenD); 1,500+ using Istio (Istio) | undated, roughly 2020-2022 | Vendor | [KrakenD](https://www.krakend.io/case-study/rappi/); [Istio](https://istio.io/latest/about/case-studies/rappi/) |
| Infra team | ~70 | undated | Vendor | [KrakenD](https://www.krakend.io/case-study/rappi/) |
| K8s clusters / containers | 50+ / 30,000+ | ~2022 | Vendor (Istio) | [Istio](https://istio.io/latest/about/case-studies/rappi/) |
| Deploys | 1,500+ per day | ~2022 | Vendor | same |
| Geography | 9 countries, 250+ cities; 100M+ downloads | ~2022 | Vendor | same |
| DocumentDB | 100 DBs migrated in 3 months; 2 engineers | undated | AWS | [AWS story](https://go.aws/3orujSn) |

### Inferences
- The two developer counts (750+ vs 1,500+) likely reflect different dates or different scopes (developers using the platform vs software engineers). Neither source is dated precisely.

### Gaps
- No current (2025-2026) count of engineers, services or RPS.
- No 2025 or 2026 Black Friday figures were found.

## 6. Lessons a Go backend engineer can turn into rules (derived from the above)

### Takeaway
These are inferences from documented Rappi choices, not rules Rappi has published.

### Cited Findings
- Cross-cutting concerns live in the platform (gateway JWT validation, mesh rate limits, timeouts, circuit breakers). — [KrakenD](https://www.krakend.io/case-study/rappi/); [Istio](https://istio.io/latest/about/case-studies/rappi/)
- Rappi replaced an in-house mesh with Istio once it outgrew its maintenance budget. — [Istio](https://istio.io/latest/about/case-studies/rappi/)
- Database migrations put API compatibility first to avoid code churn ("without changing much code"). — [AWS DocumentDB story](https://go.aws/3orujSn)
- Asynchronous messaging via Kafka/SQS is a core competence in Go roles. — [Senior Backend Go posting](https://jobs.foundationcapital.com/companies/rappi-2/jobs/30739533-senior-backend-developer-go)

### Inferences
- Rule: keep auth, rate limiting, retries and timeouts out of Go service code when a gateway or mesh owns them. Make each service configurable per-service at the platform layer instead.
- Rule: prefer buying or adopting standard infrastructure (KrakenD, Istio, managed databases) over home-grown versions once maintenance cost exceeds the value.
- Rule: isolate data per country and per line of business to contain blast radius in multi-country setups.
- Rule: generate repetitive config (gateway endpoints) from templates in CI rather than hand-editing it. Note, though, that a 35,000-line Jinja template is itself a smell to avoid.
- Rule: design Go services for asynchronous, event-driven integration (Kafka/SQS), with observability wired in from day one.
- Caution: "nanoservices" at 3,500+ services led Rappi to a mesh and heavy platform tooling. A small team should not copy the granularity.

### Gaps
- No Rappi-authored engineering principles, postmortems or Go style guide are public to validate these rules.
