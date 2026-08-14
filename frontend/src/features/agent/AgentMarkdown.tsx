import { useMemo, type ReactNode } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

import { CodeBlock } from "@/features/agent/CodeBlock";

interface AgentMarkdownProps {
  /** Raw markdown text (may be a partial stream — unclosed fences simply
   *  render as code until the closing fence arrives). */
  content: string;
  /** Whether a terminal session is available for "insert". */
  canInsert: boolean;
  /** Insert a code block into the active terminal session. */
  onInsert: (code: string) => void;
}

/**
 * Renders agent output as markdown. Fenced code blocks reuse CodeBlock (with
 * Copy / Insert-to-Terminal actions); everything else gets minimal Tailwind
 * typography — there is no @tailwindcss/typography plugin in this project.
 */
export function AgentMarkdown({ content, canInsert, onInsert }: AgentMarkdownProps) {
  const components = useMemo(
    () => ({
      // Block code: the <pre> wrapper is replaced entirely by CodeBlock; the
      // inner <code> renderer is never invoked for block content.
      pre: ({ children }: { children?: ReactNode }) => (
        <CodeBlock code={nodeText(children)} canInsert={canInsert} onInsert={onInsert} />
      ),
      code: ({ children }: { children?: ReactNode }) => (
        <code className="rounded-[4px] border border-border/60 bg-background/80 px-1 py-0.5 font-mono text-[0.85em]">
          {children}
        </code>
      ),
      p: ({ children }: { children?: ReactNode }) => <p className="my-1 leading-relaxed">{children}</p>,
      h1: ({ children }: { children?: ReactNode }) => (
        <h1 className="mb-1 mt-2 text-base font-semibold first:mt-0">{children}</h1>
      ),
      h2: ({ children }: { children?: ReactNode }) => (
        <h2 className="mb-1 mt-2 text-sm font-semibold first:mt-0">{children}</h2>
      ),
      h3: ({ children }: { children?: ReactNode }) => (
        <h3 className="mb-1 mt-1.5 text-sm font-semibold first:mt-0">{children}</h3>
      ),
      h4: ({ children }: { children?: ReactNode }) => (
        <h4 className="mb-1 mt-1.5 text-sm font-semibold first:mt-0">{children}</h4>
      ),
      ul: ({ children }: { children?: ReactNode }) => <ul className="my-1 list-disc pl-5">{children}</ul>,
      ol: ({ children }: { children?: ReactNode }) => <ol className="my-1 list-decimal pl-5">{children}</ol>,
      li: ({ children }: { children?: ReactNode }) => <li className="my-0.5 leading-relaxed">{children}</li>,
      a: ({ children, href }: { children?: ReactNode; href?: string }) => (
        <a href={href} target="_blank" rel="noreferrer" className="text-primary underline underline-offset-2">
          {children}
        </a>
      ),
      blockquote: ({ children }: { children?: ReactNode }) => (
        <blockquote className="my-1 border-l-2 border-border pl-2 text-muted-foreground">{children}</blockquote>
      ),
      hr: () => <hr className="my-2 border-border" />,
      table: ({ children }: { children?: ReactNode }) => (
        <div className="my-1 overflow-x-auto">
          <table className="w-full border-collapse text-xs">{children}</table>
        </div>
      ),
      th: ({ children }: { children?: ReactNode }) => (
        <th className="border border-border bg-background/60 px-2 py-1 text-left font-medium">{children}</th>
      ),
      td: ({ children }: { children?: ReactNode }) => (
        <td className="border border-border px-2 py-1 align-top">{children}</td>
      ),
    }),
    [canInsert, onInsert],
  );

  return (
    <div className="agent-md min-w-0 break-words text-sm">
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={components}>
        {content}
      </ReactMarkdown>
    </div>
  );
}

/** nodeText flattens a rendered markdown node tree to plain text (used to
 *  extract the source of a fenced code block). */
function nodeText(node: ReactNode): string {
  if (node == null || node === false || node === true) return "";
  if (typeof node === "string" || typeof node === "number") return String(node);
  if (Array.isArray(node)) return node.map(nodeText).join("");
  const el = node as { props?: { children?: ReactNode } };
  if (el.props?.children !== undefined) return nodeText(el.props.children);
  return "";
}
