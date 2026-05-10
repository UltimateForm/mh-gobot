# Skirmish Scoring System

## Quick Summary (TLDR)

Your score changes based on **wins, losses, and who you fight**.

- **Win a round?** Get a bonus based on your performance and who you beat
- **Lose the match?** Get penalized based on your score and how much you played
- **Fight tougher opponents?** Bigger rewards and lighter penalties
- **Outnumbered?** You lose less if your team was smaller

Your final score = all round scores + match bonus/penalty. Higher-ranked players gain points slower but lose them faster - making rank harder to climb but easier to drop.

## How Scoring Works

The scoring system rewards:

1. **Winning rounds with strong performance** - More kills/assists = bigger bonus
2. **Fighting ranked opponents** - Beating skilled players is worth more
3. **Being reliable** - Playing the full match matters more than joining at the end
4. **Overcoming odds** - Winning when outnumbered gives bigger rewards

## Per-Round Win Bonuses

When your team wins a round, you get a bonus that depends on:

**Your Performance**
- How many kills, assists, and objective points you earned that round
- Stronger performance = larger bonus

**Team Imbalance**
- If your team is outnumbered, you get a bigger bonus for winning
- Example: 5 players beating 8 players get a 1.6x boost

**Your Opponents' Skill**
- The better your victims, the more the bonus is worth
- Killing a top-ranked player gives way more value than killing a low-ranked one
- Beating similarly-ranked players: 1x bonus (the baseline/normal amount)
- Top-ranked player beating low-ranked players: less than 1x bonus

**Your Overall Rank**
- Lower-ranked players gain points faster than top players
- This prevents the top from getting too far ahead and keeps competition fair
- You still climb, just slower as you get better

## Match Loss Penalties

When a match ends, the losing team loses points. The penalty depends on:

**Your Score**
- Higher-ranked players lose more points (up to ~4x the base amount)
- Lower-ranked players lose less
- You can never lose more points than you currently have

**How Much You Played**
- Only participated in 5 of 10 rounds? You lose only half the penalty
- Played the whole match? You get the full penalty
- This scales punishment fairly - you're penalized based on how much you contributed to the loss

**Team Imbalance**
- If your team was outnumbered, you lose less
- Example: 3 players losing to 8 lose significantly less than losing 5v5

## Winning the Match

When your team wins the match, you also get a bonus from the points the losing team lost - distributed among winners based on how much each of you played. Players who were there for the whole match get bigger bonuses.

## Your Final Score

Your final match score = all round bonuses + match bonus/penalty

**Example:**
- You earned 500 points across 10 rounds
- Your team lost, penalty was -100
- **Your final score: 400**

After the match ends, you'll see a summary showing:
- Kills, deaths, assists
- What % of the match you participated in
- Your total score change (positive or negative)

## Leaving Early

If you disconnect from a losing team, you're penalized immediately based on:
- Your current rank
- How many rounds you played
- If your team was outnumbered

This is to prevent people from rage-quitting without consequence, while still being fair to players with legitimate disconnects.

## Pausing Your Rank

You can opt out of ranking changes at any time using the in-game `!rr` command:

- `!rr` - check whether your scoring is currently active or paused
- `!rr off` - pause ranking; your kills/deaths/assists still track but your rank stays locked
- `!rr on` - resume ranking; future round bonuses and match penalties apply again

While paused you won't gain or lose rank from anything - useful if you're warming up, goofing around, or playing modes you don't want counted. Admins can disable the command server-wide or override your setting.
