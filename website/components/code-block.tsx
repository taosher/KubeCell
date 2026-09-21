import { CopyButton } from "@/components/copy-button";
import { highlightLine, tokenClassName, type Language } from "@/lib/highlight";
import { cn } from "@/lib/utils";

export type CodeBlockProps = {
  code: string;
  language?: Language;
  filename?: string;
  className?: string;
  showLineNumbers?: boolean;
  maxHeight?: string;
};

export function CodeBlock({
  code,
  language = "yaml",
  filename,
  className,
  showLineNumbers = false,
  maxHeight,
}: CodeBlockProps) {
  const lines = code.replace(/\n$/, "").split("\n");
  const raw = lines.join("\n");

  return (
    <div
      className={cn(
        "group relative overflow-hidden rounded-xl border border-white/10 bg-[#070b14] shadow-[0_20px_60px_-30px_rgba(15,40,90,0.9)]",
        className
      )}
    >
      <div className="flex items-center justify-between border-b border-white/5 bg-white/[0.02] px-3 py-2">
        <div className="flex items-center gap-2">
          <span className="flex gap-1.5">
            <span className="h-2.5 w-2.5 rounded-full bg-red-500/70" />
            <span className="h-2.5 w-2.5 rounded-full bg-amber-400/70" />
            <span className="h-2.5 w-2.5 rounded-full bg-emerald-500/70" />
          </span>
          {filename ? (
            <span className="ml-2 font-mono text-xs text-slate-400">{filename}</span>
          ) : (
            <span className="ml-2 font-mono text-xs uppercase tracking-wider text-slate-500">
              {language}
            </span>
          )}
        </div>
        <CopyButton value={raw} />
      </div>
      <div className="overflow-auto" style={maxHeight ? { maxHeight } : undefined}>
        <pre className="p-4 text-[13px] leading-6">
          <code className="font-mono">
            {lines.map((line, index) => (
              <span key={index} className="grid grid-cols-[auto_1fr] gap-4">
                {showLineNumbers ? (
                  <span className="select-none text-right text-slate-600">{index + 1}</span>
                ) : (
                  <span className="hidden" />
                )}
                <span className="whitespace-pre text-slate-200">
                  {highlightLine(line, language).map((token, tokenIndex) => (
                    <span key={tokenIndex} className={tokenClassName(token.type)}>
                      {token.text}
                    </span>
                  ))}
                </span>
              </span>
            ))}
          </code>
        </pre>
      </div>
    </div>
  );
}
