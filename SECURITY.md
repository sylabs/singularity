# Security Policy

SingularityCE follows the Sylabs Security Policy available at:

<https://sylabs.io/security-policy/>

## Reporting a Vulnerability

If you have found a vulnerability in SingularityCE, please review the policy
linked above and then contact <security@sylabs.io>

PGP encrypted email is accepted, with key details at the link above.

## LLM / AI Discovery of Security Issues

Like other open source projects, SingularityCE increasingly receives reports of
potential security vulnerabilities that have been wholly or mostly generated
using LLMs / AI tooling. While these tools can identify valid security issues,
they can also generate false positives. The ease of generation of (duplicate)
reports raises the burden on maintainers.

To minimise the impact on the project, please:

* Disclose to what extent the findings were LLM generated, and which model(s)
  were used. This information will allow us to improve internal procedures.
* Ensure that the model has followed the guidance in our [AGENTS.md](AGENTS.md)
  file regarding common false positives and required security analysis context.
* Provide context around the reported vulnerability that demonstrates
  understanding of how SingularityCE is used, and how it differs from other
  container runtimes, where appropriate.
* Be as concise as possible. Edit excessively detailed LLM output &
  over-complicated reproducer scripts.
* Answer any queries from maintainers yourself, rather than replying with direct
  LLM output.
