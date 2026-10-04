import { useEffect, useRef, useState } from "react";
import { draftOrder, OrderDraftResponse, OrderQuestion } from "../api/assistants";
import { useLocale } from "../i18n";
import { DraftLine, FormFill, formFills, lineFromProduct } from "./orderDraft.mjs";
import { formatDay } from "./orderStage.mjs";
import { HelperUnavailableNote } from "./HelperUnavailableNote";
import { isHelperUnavailable } from "./helperAvailability.mjs";
import { appendSpoken, speechErrorMessage, speechLanguage, speechRecognitionCtor } from "./speechInput.mjs";

const MAX_TEXT = 2000;

type Props = { token: string; onFill: (fill: FormFill, neededBy: string | null) => void; onUnavailable?: () => void };

function QuestionRow({ question, onAnswer, onSkip }: { question: OrderQuestion; onAnswer: (line: DraftLine) => void; onSkip: () => void }) {
  const { t } = useLocale();
  const [productId, setProductId] = useState(question.options.length === 1 ? question.options[0].id : "");
  const [quantity, setQuantity] = useState(question.suggestedQuantity ? String(question.suggestedQuantity) : "");
  const product = question.options.find((p) => p.id === productId);
  const answerable = question.kind === "choose_product" || question.kind === "quantity";
  const heading =
    question.kind === "choose_product" ? t("Which one did you mean?")
      : question.kind === "quantity" ? t("How many?")
        : question.kind === "not_in_catalog" ? t("Not in your product list. Add it to the totals by hand if you need it.")
          : question.kind === "previous_not_found" ? t("We could not find that earlier order. Type the items instead.")
            : t("Choose the delivery date on the form.");
  const line = product ? lineFromProduct(product, quantity, question.sourceText) : null;

  return (
    <li className="sm-helper-question" role="group" aria-label={heading}>
      <p className="sm-helper-question-title">{heading}</p>
      {question.sourceText && <p className="sm-helper-quote muted">“{question.sourceText}”</p>}
      {answerable && (
        <div className="sm-helper-answer">
          {question.options.length > 1 ? (
            <select aria-label={t("Product")} value={productId} onChange={(e) => setProductId(e.target.value)}>
              <option value="">{t("Choose a product")}</option>
              {question.options.map((p) => <option key={p.id} value={p.id}>{`${p.name} · ${p.pack} ${p.unitsPerPack}`}</option>)}
            </select>
          ) : product && <span className="sm-helper-product">{`${product.name} · ${product.pack} ${product.unitsPerPack}`}</span>}
          <input aria-label={t("Quantity")} type="number" inputMode="numeric" min="1" max="999" step="1" value={quantity} onChange={(e) => setQuantity(e.target.value)} />
          <button type="button" className="sm-btn-secondary" disabled={!line} onClick={() => line && onAnswer(line)}>{t("Add line")}</button>
        </div>
      )}
      <button type="button" className="sm-helper-link" onClick={onSkip}>{answerable ? t("Skip") : t("OK")}</button>
    </li>
  );
}

export function OrderTextHelper({ token, onFill, onUnavailable }: Props) {
  const { t, locale } = useLocale();
  const [text, setText] = useState("");
  // ST-3: speak the order. The browser turns speech into text in the box; the manager checks it before "Fill in for me".
  const Recognition = speechRecognitionCtor(typeof window === "undefined" ? undefined : window);
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const recognition = useRef<any>(null);
  const [listening, setListening] = useState(false);
  const [heard, setHeard] = useState("");
  const [speechError, setSpeechError] = useState("");
  useEffect(() => () => recognition.current?.abort(), []);

  function toggleSpeech() {
    if (listening) { recognition.current?.stop(); return; }
    if (!Recognition) return;
    const r = new Recognition();
    r.lang = speechLanguage(locale);
    r.interimResults = true;
    r.continuous = true;
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    r.onresult = (event: any) => {
      let interim = "";
      for (let i = event.resultIndex; i < event.results.length; i++) {
        const said = event.results[i][0]?.transcript || "";
        if (event.results[i].isFinal) setText((current) => appendSpoken(current, said).slice(0, MAX_TEXT));
        else interim += said;
      }
      setHeard(interim.trim());
    };
    r.onerror = (event: { error: string }) => setSpeechError(speechErrorMessage(event.error));
    r.onend = () => { setListening(false); setHeard(""); recognition.current = null; };
    setSpeechError("");
    recognition.current = r;
    try { r.start(); setListening(true); } catch { setListening(false); }
  }
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [unavailable, setUnavailable] = useState(false);
  const [result, setResult] = useState<OrderDraftResponse | null>(null);
  const [lines, setLines] = useState<DraftLine[]>([]);
  const [questions, setQuestions] = useState<OrderQuestion[]>([]);
  const [includePrevious, setIncludePrevious] = useState(true);

  async function read() {
    if (!text.trim() || busy) return;
    setBusy(true);
    setError("");
    try {
      const body = await draftOrder(token, text.trim());
      setResult(body);
      setLines(body.lines);
      setQuestions(body.questions);
      setIncludePrevious(true);
    } catch (err) {
      setResult(null);
      if (isHelperUnavailable(err)) {
        // The helper is switched off: say so once, calmly. The order form is untouched and still works.
        setUnavailable(true);
        onUnavailable?.();
      } else {
        setError(t("We could not read that order. Check your connection or fill in the form below."));
      }
    } finally {
      setBusy(false);
    }
  }

  const fills = result ? formFills(lines, result.previousOrder, includePrevious) : [];
  const temperatureLabel = (value: string) => (value === "chilled" ? t("Chilled") : t("Ambient"));

  return (
    <section className="sm-form-card sm-helper-card" aria-labelledby="order-text-heading">
      <h2 id="order-text-heading" className="sm-form-card-title">{t("Paste, type or speak your order")}</h2>
      <p className="sm-form-card-sub muted">{t("Write it the way you would tell a colleague, for example “same as last Tuesday” or “rice 10 bags, oil 24 bottles”. We fill in the form for you to check. Nothing is sent until you press Submit Order.")}</p>
      {unavailable ? <HelperUnavailableNote /> : <>
      <div className="sm-field">
        <label htmlFor="order-text">{t("Your order in your own words")}</label>
        <textarea id="order-text" rows={3} maxLength={MAX_TEXT} value={text} onChange={(e) => setText(e.target.value)} />
        {listening && <p className="sm-field-hint" role="status" aria-live="polite">{heard ? `${t("Hearing")}: “${heard}”` : t("Listening… say the items and quantities, then tap Stop.")}</p>}
        {speechError && <p className="status-bad sm-form-error" role="alert">{t(speechError)}</p>}
      </div>
      <div className="sm-helper-actions">
        {Recognition && (
          <button type="button" className="sm-btn-secondary" aria-pressed={listening} onClick={toggleSpeech} disabled={busy}>
            {listening ? `■ ${t("Stop")}` : `🎤 ${t("Speak your order")}`}
          </button>
        )}
        <button type="button" className="tap primary" onClick={() => { recognition.current?.stop(); void read(); }} disabled={busy || !text.trim()}>
          {busy ? t("Reading your order…") : t("Fill in for me")}
        </button>
        {result && <button type="button" className="sm-helper-link" onClick={() => { setResult(null); setLines([]); setQuestions([]); setText(""); }}>{t("Clear")}</button>}
      </div>
      {error && <p className="status-bad sm-form-error" role="alert">{error}</p>}
      </>}

      {result && (
        <div className="sm-helper-result" aria-live="polite">
          {result.previousOrder && (
            <label className="sm-helper-previous">
              <input type="checkbox" checked={includePrevious} onChange={(e) => setIncludePrevious(e.target.checked)} />
              <span>{`${t("Include the totals from order")} ${result.previousOrder.orderRef} (${formatDay(result.previousOrder.placedOn)} · ${result.previousOrder.orderUnits} ${t("units")} · ${temperatureLabel(result.previousOrder.temperatureRequirement)})`}</span>
            </label>
          )}
          {result.previousOrder && <p className="sm-field-hint muted">{t("Earlier orders only kept totals, not item lines, so the totals are copied.")}</p>}

          {lines.length > 0 && (
            <ul className="sm-helper-lines" aria-label={t("Order lines")}>
              {lines.map((line, i) => (
                <li key={`${line.productId}-${i}`}>
                  <span>{line.name}</span>
                  <span className="muted">{`${line.quantity} × ${line.pack} ${line.unitsPerPack} · ${line.weightKg} kg · ${temperatureLabel(line.temperature)}`}</span>
                  <button type="button" className="sm-helper-link" aria-label={`${t("Remove")} ${line.name}`} onClick={() => setLines(lines.filter((_, j) => j !== i))}>{t("Remove")}</button>
                </li>
              ))}
            </ul>
          )}

          {questions.length > 0 && (
            <ul className="sm-helper-questions" aria-label={t("Please check")}>
              {questions.map((q, i) => (
                <QuestionRow
                  key={`${q.kind}-${q.sourceText}-${i}`}
                  question={q}
                  onAnswer={(line) => { setLines([...lines, line]); setQuestions(questions.filter((_, j) => j !== i)); }}
                  onSkip={() => setQuestions(questions.filter((_, j) => j !== i))}
                />
              ))}
            </ul>
          )}

          {fills.length > 1 && <p className="sm-field-hint muted">{t("Ambient and chilled goods are separate orders. Fill in one, submit it, then fill in the other.")}</p>}
          <div className="sm-helper-fills">
            {fills.map((fill) => (
              <button key={fill.temperature} type="button" className="sm-btn-secondary" onClick={() => onFill(fill, result.neededBy)}>
                {`${t("Use for the order form")}: ${temperatureLabel(fill.temperature)} · ${fill.orderUnits} ${t("units")} · ${fill.orderWeightKg} kg`}
              </button>
            ))}
          </div>
          {fills.length === 0 && questions.length === 0 && <p className="sm-field-hint muted">{t("Nothing to fill in yet. Try writing the items with quantities.")}</p>}
        </div>
      )}
    </section>
  );
}
