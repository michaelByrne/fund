-- name: GetFundStats :one
SELECT COALESCE(SUM(amount_cents), 0)::INTEGER AS total_donated,
       COUNT(*)                                AS total_donations,
       CASE
           WHEN COUNT(*) > 0 THEN COALESCE(SUM(amount_cents), 0) / COUNT(*)
           ELSE 0
           END                                 AS average_donation,
       COUNT(DISTINCT donor_id)                AS total_donors
FROM donation
         JOIN member m ON donation.donor_id = m.id
         LEFT JOIN donation_payment dp ON donation.id = dp.donation_id
WHERE fund_id = $1;


-- name: GetMonthlyDonationTotalsForFund :many
SELECT sum(amount_cents)               as total_donated,
       date_trunc('month', dp.created) as month
FROM donation d
         JOIN donation_payment dp on d.id = dp.donation_id
WHERE fund_id = $1
  AND active = true
  AND d.recurring = true
group by dp.created;