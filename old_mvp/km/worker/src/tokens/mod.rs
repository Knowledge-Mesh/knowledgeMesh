/// Estimate token count from text using character length heuristic.
///
/// Anthropic's tokeniser averages roughly 4 characters per token for English.
/// This is intentionally conservative — overestimating slightly is better than
/// undercharging a buyer.
const CHARS_PER_TOKEN: f64 = 3.8;

pub fn estimate_tokens(text: &str) -> u32 {
    let chars = text.len() as f64;
    (chars / CHARS_PER_TOKEN).ceil() as u32
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn short_text() {
        // "Hello world" = 11 chars → ~3 tokens
        assert!(estimate_tokens("Hello world") >= 2);
        assert!(estimate_tokens("Hello world") <= 5);
    }

    #[test]
    fn empty_text() {
        assert_eq!(estimate_tokens(""), 0);
    }

    #[test]
    fn longer_text() {
        let text = "a".repeat(3800);
        // 3800 chars / 3.8 = 1000 tokens
        assert_eq!(estimate_tokens(&text), 1000);
    }
}
