import Markdown, { defaultUrlTransform } from "react-markdown";
import remarkGfm from "remark-gfm";
import { markMentions } from "./messenger-parts";

// Agent replies are Markdown. Mentions of coworkers are highlighted, and links
// to host files open in the Mace viewer, since file:// links do nothing on a phone.
export default function AgentMessage({
  text,
  handles = [],
  onFile,
}: {
  text: string;
  handles?: string[];
  onFile?: (path: string) => void;
}) {
  return (
    <Markdown
      remarkPlugins={[remarkGfm]}
      skipHtml
      urlTransform={(url) =>
        url.startsWith("mace-mention:") || url.startsWith("file://")
          ? url
          : defaultUrlTransform(url)
      }
      components={{
        a: ({ children, href = "" }) => {
          if (href.startsWith("mace-mention:"))
            return <span className="mention">{children}</span>;
          if (href.startsWith("file://") && onFile) {
            const path = decodeURI(href.slice("file://".length));
            return (
              <button type="button" className="file-link" onClick={() => onFile(path)} title={path}>
                {children}
              </button>
            );
          }
          return (
            <a href={href} target="_blank" rel="noopener noreferrer">
              {children}
            </a>
          );
        },
      }}
    >
      {markMentions(text, handles)}
    </Markdown>
  );
}
