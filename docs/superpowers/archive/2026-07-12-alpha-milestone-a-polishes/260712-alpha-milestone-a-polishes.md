1. Create small animation to run during AI 재평가.
   Currently, there's no animation, so it can feel like the 재평가 ran into a blocker if there's a particularly long 평가 (i.e., if it takes 10 seconds at 5/50).
2. If I run AI 재평가 on 데일리 브리핑, navigate to 전체 공고 while the rating is in progress, and then use back/forward navigation to return to that same 데일리 브리핑 page, the AI 평가 correctly continues on the server, but the restored page's progress UI breaks. The other page does not need to display the progress. When I return to the original 데일리 브리핑 history entry, its 재평가 button should still be grayed out, and the user should again see this text
   ```
   AI로 다시 분석하는 중이에요 — 여러 공고를 한 번에 살펴보고 있어요. ☕
   공고 27/41 분석 중...
   ```
   with the counter resuming from the current progress and continuing to update (don't forget the animation).
   If the evaluation finishes while the user is on another page, returning to the original page should automatically refresh it once so the new scores are visible, then show a one-time completion message: "AI 평가가 완료됐어요. 새로운 평가 결과를 반영했습니다."
   We're using server-sent events for the counter btw, right?
3. Also, let's change the "재평가" button's text to "AI 평가".
   - When all the listings are already rated and I press the button again, the message appears for a bit but then disappears and no rating is done. Is this intentional? If so, that's good (limits token use), but let's add a message that says "이미 모든 공고가 AI로 평가됐습니다" or sth like that.
4. Also, only the AI 분석 chip/panel within each listing should have its own unique standout color to distinguish it from the other listing metadata. Do not tint or otherwise recolor the surrounding listing card.
   - Approved direction: Electric indigo.
   - Light theme: chip `#eeeafe`, panel `#f3f0ff`, border `#c9bdfa`, text `#3f307c`, accent/focus `#6748c7`.
   - Dark theme: chip `#29233f`, panel `#211c34`, border `#5b4d8d`, text `#ede8ff`, accent/focus `#b7a7ff`.
   - Keep the AI chip's existing shape, spacing, keyboard behavior, and mobile touch target. Use the indigo accent for its visible focus ring.
5. Also, Demoday should be disabled by default. There is only one existing user during this alpha, so apply the new default to the existing profile as well; I can re-enable it manually if needed.
