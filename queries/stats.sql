-- name: GetFundStats :one
SELECT sum(amount_cents)            as total_donated,
       count(*)                     as total_donations,
       sum(amount_cents) / count(*) as average_donation,
       count(distinct donor_id)     as total_donors
FROM donation
         JOIN member m on donation.donor_id = m.id
         JOIN donation_payment dp on donation.id = dp.donation_id
WHERE fund_id = $1;

-- name: GetMonthlyDonationTotalsForFund :many
SELECT SUM(amount_cents)               AS total_donated,
       date_trunc('month', dp.created) AS month
FROM donation d
         JOIN donation_payment dp ON d.id = dp.donation_id
WHERE d.fund_id = $1
  AND d.recurring = true
GROUP BY date_trunc('month', dp.created)
ORDER BY month;
