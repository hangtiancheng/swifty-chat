/**
 * Copyright (c) 2026 hangtiancheng
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in
 * all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

import { HelpCircle } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { cn } from "@/lib/utils";
import type { AgentQuestionItem, Question } from "@/service/agent-schemas";
import useAgentStore from "@/store/agent";

/** Sentinel for the free-text row that every question gets. */
const OTHER = "__other__";

interface Draft {
  selected: string[];
  other: string;
  useOther: boolean;
}

const emptyDraft = (): Draft => ({ selected: [], other: "", useOther: false });

/** Multi-select joins the chosen labels; single-select answers with the one
 * pick, or with the free text when "Other" is in use. */
function buildAnswer(question: Question, draft: Draft): string {
  const parts = [...draft.selected];
  if (draft.useOther && draft.other.trim() !== "") {
    parts.push(draft.other.trim());
  }
  if (question.multiSelect) return parts.join(", ");
  if (draft.useOther) return draft.other.trim();
  return parts[0] ?? "";
}

/** Swiftx blocks on this card the same way it blocks on a permission prompt. */
export function AgentQuestionCard({ item }: { item: AgentQuestionItem }) {
  const answerQuestions = useAgentStore((state) => state.answerQuestions);
  const [drafts, setDrafts] = useState<Record<number, Draft>>({});

  const draftAt = (index: number) => drafts[index] ?? emptyDraft();
  const patch = (index: number, next: Partial<Draft>) =>
    setDrafts((current) => ({
      ...current,
      [index]: { ...(current[index] ?? emptyDraft()), ...next },
    }));

  if (item.answered) {
    return <p className="text-muted-foreground text-xs">Answered</p>;
  }

  const submit = () => {
    const answers: Record<string, string> = {};
    item.questions.forEach((question, index) => {
      answers[question.question] = buildAnswer(question, draftAt(index));
    });
    answerQuestions(item.id, answers);
  };

  return (
    <section
      aria-label="Question from Swiftx"
      className="border-primary/30 bg-card max-w-[85%] rounded-lg border px-3 py-2.5 shadow-sm"
    >
      <div className="text-foreground flex items-center gap-1.5 text-xs font-semibold">
        <HelpCircle className="text-primary size-3.5" />
        Swiftx needs your input
      </div>

      {item.questions.map((question, index) => (
        <QuestionRow
          key={`${item.id}_${index}`}
          question={question}
          draft={draftAt(index)}
          onChange={(next) => patch(index, next)}
        />
      ))}

      <div className="mt-2.5 flex justify-end">
        <Button size="sm" onClick={submit}>
          Submit
        </Button>
      </div>
    </section>
  );
}

interface QuestionRowProps {
  question: Question;
  draft: Draft;
  onChange: (next: Partial<Draft>) => void;
}

function QuestionRow({ question, draft, onChange }: QuestionRowProps) {
  const rowClass = (active: boolean) =>
    cn(
      "flex cursor-pointer items-start gap-2 rounded-md border px-2 py-1.5 transition-colors",
      active
        ? "border-primary/40 bg-primary/5"
        : "border-transparent hover:bg-muted/60",
    );

  const optionLabel = (label: string, description: string) => (
    <span className="min-w-0">
      <span className="text-foreground text-xs">{label}</span>
      {description && (
        <span className="text-muted-foreground ml-1.5 text-xs">
          {description}
        </span>
      )}
    </span>
  );

  const freeText = (
    <Input
      value={draft.other}
      placeholder="Type a custom answer…"
      onFocus={() =>
        onChange({
          useOther: true,
          selected: question.multiSelect ? draft.selected : [],
        })
      }
      onChange={(event) =>
        onChange({ other: event.target.value, useOther: true })
      }
      className="h-7 min-w-0 flex-1 text-xs"
    />
  );

  return (
    <fieldset className="mt-2">
      <legend className="text-foreground mb-1 text-xs font-medium">
        {question.question || question.header}
        {question.multiSelect && (
          <span className="text-muted-foreground ml-1.5 font-normal">
            (select all that apply)
          </span>
        )}
      </legend>

      {question.multiSelect ? (
        <div className="flex flex-col gap-0.5">
          {question.options.map((option) => (
            <label
              key={option.label}
              className={rowClass(draft.selected.includes(option.label))}
            >
              <Checkbox
                checked={draft.selected.includes(option.label)}
                onCheckedChange={(checked) =>
                  onChange({
                    selected: checked
                      ? [...draft.selected, option.label]
                      : draft.selected.filter(
                          (label) => label !== option.label,
                        ),
                  })
                }
                className="mt-0.5"
              />
              {optionLabel(option.label, option.description)}
            </label>
          ))}
          <label className={rowClass(draft.useOther)}>
            <Checkbox
              checked={draft.useOther}
              onCheckedChange={(checked) =>
                onChange({ useOther: Boolean(checked) })
              }
              className="mt-0.5"
            />
            <span className="text-muted-foreground shrink-0 text-xs">
              Other:
            </span>
            {freeText}
          </label>
        </div>
      ) : (
        <RadioGroup
          value={draft.useOther ? OTHER : (draft.selected[0] ?? "")}
          onValueChange={(value) =>
            value === OTHER
              ? onChange({ useOther: true, selected: [] })
              : onChange({ useOther: false, selected: [String(value)] })
          }
          className="gap-0.5"
        >
          {question.options.map((option) => (
            <label
              key={option.label}
              className={rowClass(draft.selected[0] === option.label)}
            >
              <RadioGroupItem value={option.label} className="mt-0.5" />
              {optionLabel(option.label, option.description)}
            </label>
          ))}
          <label className={rowClass(draft.useOther)}>
            <RadioGroupItem value={OTHER} className="mt-0.5" />
            <span className="text-muted-foreground shrink-0 text-xs">
              Other:
            </span>
            {freeText}
          </label>
        </RadioGroup>
      )}
    </fieldset>
  );
}
